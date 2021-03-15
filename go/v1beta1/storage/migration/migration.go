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

	"github.com/elastic/go-elasticsearch/v7"
	"go.uber.org/zap"
)

func NewESMigrator(logger *zap.Logger, client *elasticsearch.Client, indexManager IndexManager, sleep timeSleep) *ESMigrator {
	return &ESMigrator{
		client:       client,
		logger:       logger,
		indexManager: indexManager,
		sleep:        sleep,
	}
}

func (e *ESMigrator) Migrate(ctx context.Context, migration *Migration) error {
	log := e.logger.Named("Migrate").With(zap.String("indexName", migration.Index))
	log.Info("Starting migration")

	res, err := e.client.Indices.GetSettings(e.client.Indices.GetSettings.WithContext(ctx), e.client.Indices.GetSettings.WithIndex(migration.Index))
	if err := getErrorFromESResponse(res, err); err != nil {
		return fmt.Errorf("error checking if write block is enabled on index: %s", err)
	}

	settingsResponse := map[string]ESSettingsResponse{}
	if err := decodeResponse(res.Body, &settingsResponse); err != nil {
		return fmt.Errorf("error decoding settings response: %s", err)
	}

	blocks := settingsResponse[migration.Index].Settings.Index.Blocks

	if !(blocks != nil && blocks.Write == "true") {
		log.Info("Placing write block on index")
		res, err = e.client.Indices.AddBlock([]string{migration.Index}, "write", e.client.Indices.AddBlock.WithContext(ctx))
		if err := getErrorFromESResponse(res, err); err != nil {
			return fmt.Errorf("error placing write block on index: %s", err)
		}

		blockResponse := &ESBlockResponse{}
		if err := decodeResponse(res.Body, blockResponse); err != nil {
			return fmt.Errorf("error decoding write block response: %s", err)
		}

		if !(blockResponse.Acknowledged && blockResponse.ShardsAcknowledged) {
			log.Error("Write block unsuccessful", zap.Any("response", blockResponse))
			return fmt.Errorf("unable to block writes for index: %s", migration.Index)
		}
	}

	newIndexName := e.indexManager.IncrementIndexVersion(migration.Index)
	err = e.indexManager.CreateIndex(ctx, &IndexInfo{
		Index:        newIndexName,
		Alias:        migration.Alias,
		DocumentKind: migration.DocumentKind,
	}, true)
	if err != nil {
		return fmt.Errorf("error creating target index: %s", err)
	}

	reindexReq := &ESReindex{
		Conflicts:   "proceed",
		Source:      &ReindexFields{Index: migration.Index},
		Destination: &ReindexFields{Index: newIndexName, OpType: "create"},
	}
	reindexBody := encodeRequest(reindexReq)
	log.Info("Starting reindex")
	res, err = e.client.Reindex(
		reindexBody,
		e.client.Reindex.WithContext(ctx),
		e.client.Reindex.WithWaitForCompletion(false))
	if err := getErrorFromESResponse(res, err); err != nil {
		return fmt.Errorf("error initiating reindex: %s", err)
	}
	taskCreationResponse := &ESTaskCreationResponse{}

	if err := decodeResponse(res.Body, taskCreationResponse); err != nil {
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
		if err := decodeResponse(res.Body, task); err != nil {
			log.Warn("error decoding task response", zap.Error(err))
			continue
		}

		if task.Completed {
			reindexCompleted = true
			log.Info("Reindex completed")

			break
		}

		log.Info("Task incomplete, waiting before polling again", zap.String("taskId", taskCreationResponse.Task))
		e.sleep(time.Second * 10)
	}

	if !reindexCompleted {
		return fmt.Errorf("reindex did not complete after %d polls", pollingAttempts)
	}

	res, err = e.client.Delete(".tasks", taskCreationResponse.Task, e.client.Delete.WithContext(ctx))
	if err := getErrorFromESResponse(res, err); err != nil {
		log.Warn("Error deleting task document", zap.Error(err), zap.String("taskId", taskCreationResponse.Task))
	}

	aliasReq := &ESIndexAliasRequest{
		Actions: []ESActions{
			{
				Remove: &ESIndexAlias{
					Index: migration.Index,
					Alias: migration.Alias,
				},
			},
			{
				Add: &ESIndexAlias{
					Index: newIndexName,
					Alias: migration.Alias,
				},
			},
		},
	}

	aliasReqBody := encodeRequest(aliasReq)
	log.Info("Swapping alias over to new index")
	res, err = e.client.Indices.UpdateAliases(
		aliasReqBody,
		e.client.Indices.UpdateAliases.WithContext(ctx),
	)

	if err := getErrorFromESResponse(res, err); err != nil {
		return fmt.Errorf("error occurred while swapping the alias: %s", err)
	}

	log.Info("Deleting old index")
	res, err = e.client.Indices.Delete(
		[]string{migration.Index},
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

func (e *ESMigrator) GetMigrations(ctx context.Context) ([]*Migration, error) {
	res, err := e.client.Indices.Get([]string{"_all"}, e.client.Indices.Get.WithContext(ctx))
	if err := getErrorFromESResponse(res, err); err != nil {
		return nil, err
	}

	allIndices := map[string]interface{}{}

	_ = decodeResponse(res.Body, &allIndices)
	var migrations []*Migration
	for indexName := range allIndices {
		if !strings.HasPrefix(indexName, "grafeas") {
			continue
		}

		indexParts := parseIndexName(indexName)
		latestVersion := e.indexManager.GetLatestVersionForDocumentKind(indexParts.DocumentKind)
		alias := e.indexManager.GetAliasForIndex(indexName)

		if indexParts.Version != latestVersion {
			migrations = append(migrations, &Migration{Index: indexName, DocumentKind: indexParts.DocumentKind, Alias: alias})
		}
	}

	return migrations, nil
}