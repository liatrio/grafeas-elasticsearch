// Copyright 2021 The Rode Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package migration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v7/esapi"

	"github.com/elastic/go-elasticsearch/v7"
	. "github.com/rode/grafeas-elasticsearch/go/v1beta1/storage/esutil"
	"go.uber.org/zap"
)

var (
	timeSleep = time.Sleep
)

const (
	esAllIndices         = "_all"
	esTaskIndex          = ".tasks"
	timeBetweenTaskPolls = time.Second * 10
)

type EsMigrator struct {
	client       *elasticsearch.Client
	indexManager IndexManager
	logger       *zap.Logger
}

type Migrator interface {
	GetMigrations(ctx context.Context) ([]*IndexInfo, error)
	Migrate(ctx context.Context, migration *IndexInfo) error
}

func NewEsMigrator(logger *zap.Logger, client *elasticsearch.Client, indexManager IndexManager) *EsMigrator {
	return &EsMigrator{
		client:       client,
		logger:       logger,
		indexManager: indexManager,
	}
}

func (e *EsMigrator) GetMigrations(ctx context.Context) ([]*IndexInfo, error) {
	res, err := e.client.Indices.Get([]string{esAllIndices}, e.client.Indices.Get.WithContext(ctx))
	if err := getErrorFromESResponse(res, err); err != nil {
		return nil, err
	}

	allIndices := map[string]ESIndex{}

	if err := DecodeResponse(res.Body, &allIndices); err != nil {
		return nil, err
	}

	var indicesToMigrate []*IndexInfo
	for indexName, indexValue := range allIndices {
		meta := indexValue.Mappings.Meta
		if !(strings.HasPrefix(indexName, IndexPrefix) && meta != nil && meta.Type == IndexPrefix) {
			continue
		}

		indexParts := ParseIndexName(indexName)
		latestVersion := e.indexManager.GetLatestVersionForDocumentKind(indexParts.DocumentKind)
		alias := e.indexManager.GetAliasForIndex(indexName)

		if indexParts.Version != latestVersion {
			indicesToMigrate = append(indicesToMigrate, &IndexInfo{Index: indexName, DocumentKind: indexParts.DocumentKind, Alias: alias})
		}
	}

	return indicesToMigrate, nil
}

func (e *EsMigrator) Migrate(ctx context.Context, indexInfo *IndexInfo) error {
	log := e.logger.Named("Migrate").With(zap.String("indexName", indexInfo.Index))
	log.Info("Starting migration")

	if err := e.blockWritesOnIndex(ctx, indexInfo.Index); err != nil {
		return err
	}

	newIndexName := e.indexManager.IncrementIndexVersion(indexInfo.Index)
	err := e.indexManager.CreateIndex(ctx, &IndexInfo{
		Index:        newIndexName,
		Alias:        indexInfo.Alias,
		DocumentKind: indexInfo.DocumentKind,
	}, true)
	if err != nil {
		return fmt.Errorf("error creating target index: %s", err)
	}

	if err := e.reindex(ctx, indexInfo.Index, newIndexName); err != nil {
		return err
	}

	if err := e.swapAlias(ctx, indexInfo, newIndexName); err != nil {
		return err
	}

	log.Info("Deleting old index")
	res, err := e.client.Indices.Delete(
		[]string{indexInfo.Index},
		e.client.Indices.Delete.WithContext(ctx),
	)

	if err != nil {
		return fmt.Errorf("failed to remove previous index: %s", err)
	}

	if res.IsError() && res.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to remove the previous index, status: %d", res.StatusCode)
	}

	log.Info("Migration complete")
	return nil
}

func (e *EsMigrator) blockWritesOnIndex(ctx context.Context, indexName string) error {
	log := e.logger.Named("blockWritesOnIndex").With(zap.String("indexName", indexName))

	res, err := e.client.Indices.GetSettings(e.client.Indices.GetSettings.WithContext(ctx), e.client.Indices.GetSettings.WithIndex(indexName))
	if err := getErrorFromESResponse(res, err); err != nil {
		return fmt.Errorf("error checking if write block is enabled on index: %s", err)
	}

	settingsResponse := map[string]ESSettingsResponse{}
	if err := DecodeResponse(res.Body, &settingsResponse); err != nil {
		return fmt.Errorf("error decoding settings response: %s", err)
	}

	blocks := settingsResponse[indexName].Settings.Index.Blocks

	// index already has a write block in place
	if blocks != nil && blocks.Write == "true" {
		return nil
	}

	log.Info("Placing write block on index")
	res, err = e.client.Indices.AddBlock([]string{indexName}, "write", e.client.Indices.AddBlock.WithContext(ctx))
	if err := getErrorFromESResponse(res, err); err != nil {
		return fmt.Errorf("error placing write block on index: %s", err)
	}

	blockResponse := &ESBlockResponse{}
	if err := DecodeResponse(res.Body, blockResponse); err != nil {
		return fmt.Errorf("error decoding write block response: %s", err)
	}

	if !(blockResponse.Acknowledged && blockResponse.ShardsAcknowledged) {
		log.Error("Write block unsuccessful", zap.Any("response", blockResponse))
		return fmt.Errorf("unable to block writes for index: %s", indexName)
	}

	return nil
}

func (e *EsMigrator) reindex(ctx context.Context, sourceIndex, targetIndex string) error {
	log := e.logger.Named("reindex").With(zap.String("sourceIndex", sourceIndex)).With(zap.String("targetIndex", targetIndex))
	reindexReq := &ESReindex{
		Conflicts:   "proceed",
		Source:      &ReindexFields{Index: sourceIndex},
		Destination: &ReindexFields{Index: targetIndex, OpType: "create"},
	}
	reindexBody, _ := EncodeRequest(reindexReq)
	log.Info("Starting reindex")
	res, err := e.client.Reindex(
		reindexBody,
		e.client.Reindex.WithContext(ctx),
		e.client.Reindex.WithWaitForCompletion(false))
	if err := getErrorFromESResponse(res, err); err != nil {
		return fmt.Errorf("error initiating reindex: %s", err)
	}
	taskCreationResponse := &ESTaskCreationResponse{}

	if err := DecodeResponse(res.Body, taskCreationResponse); err != nil {
		return fmt.Errorf("error decoding reindex response: %s", err)
	}
	log.Info("Reindex started", zap.String("taskId", taskCreationResponse.Task))

	reindexCompleted := false
	pollingAttempts := 10
	for i := 0; i < pollingAttempts; i++ {
		log.Info("Polling task API", zap.String("taskId", taskCreationResponse.Task))
		res, err = e.client.Tasks.Get(taskCreationResponse.Task, e.client.Tasks.Get.WithContext(ctx))
		if err := getErrorFromESResponse(res, err); err != nil {
			log.Warn("error getting task status", zap.Error(err))
			continue
		}

		task := &ESTask{}
		if err := DecodeResponse(res.Body, task); err != nil {
			log.Warn("error decoding task response", zap.Error(err))
			continue
		}

		if task.Completed {
			reindexCompleted = true
			log.Info("Reindex completed")

			break
		}

		log.Info("Task incomplete, waiting before polling again", zap.String("taskId", taskCreationResponse.Task))
		timeSleep(timeBetweenTaskPolls)
	}

	if !reindexCompleted {
		return fmt.Errorf("reindex did not complete after %d polls", pollingAttempts)
	}

	res, err = e.client.Delete(esTaskIndex, taskCreationResponse.Task, e.client.Delete.WithContext(ctx))
	if err := getErrorFromESResponse(res, err); err != nil {
		log.Warn("Error deleting task document", zap.Error(err), zap.String("taskId", taskCreationResponse.Task))
	}

	return nil
}

func (e *EsMigrator) swapAlias(ctx context.Context, sourceIndex *IndexInfo, targetIndex string) error {
	log := e.logger.Named("swapAlias").
		With(zap.String("sourceIndex", sourceIndex.Index)).
		With(zap.String("targetIndex", targetIndex))

	aliasReq := &ESIndexAliasRequest{
		Actions: []ESActions{
			{
				Remove: &ESIndexAlias{
					Index: sourceIndex.Index,
					Alias: sourceIndex.Alias,
				},
			},
			{
				Add: &ESIndexAlias{
					Index: targetIndex,
					Alias: sourceIndex.Alias,
				},
			},
		},
	}

	aliasReqBody, _ := EncodeRequest(aliasReq)
	log.Info("Swapping alias over to new index")
	res, err := e.client.Indices.UpdateAliases(
		aliasReqBody,
		e.client.Indices.UpdateAliases.WithContext(ctx),
	)

	if err := getErrorFromESResponse(res, err); err != nil {
		return fmt.Errorf("error occurred while swapping the alias: %s", err)
	}

	return nil
}

func getErrorFromESResponse(res *esapi.Response, err error) error {
	if err != nil {
		return err
	}

	if res.IsError() {
		return fmt.Errorf("response error from ES: %d", res.StatusCode)
	}
	return nil
}
