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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/elastic/go-elasticsearch/v7"
	. "github.com/rode/grafeas-elasticsearch/go/v1beta1/storage/esutil"
	"go.uber.org/zap"
)

const (
	ProjectDocumentKind    = "projects"
	OccurrenceDocumentKind = "occurrences"
	NoteDocumentKind       = "notes"

	IndexPrefix = "grafeas"
	AliasPrefix = "grafeas"
)

type IndexManager interface {
	LoadMappings(mappingsDir string) error
	CreateIndex(ctx context.Context, info *IndexInfo, checkExists bool) error

	ProjectsIndex() string
	ProjectsAlias() string

	OccurrencesIndex(projectId string) string
	OccurrencesAlias(projectId string) string

	NotesIndex(projectId string) string
	NotesAlias(projectId string) string

	IncrementIndexVersion(indexName string) string
	GetLatestVersionForDocumentKind(documentKind string) string
	GetAliasForIndex(indexName string) string
}

type EsIndexManager struct {
	logger            *zap.Logger
	client            *elasticsearch.Client
	projectMapping    *VersionedMapping
	occurrenceMapping *VersionedMapping
	noteMapping       *VersionedMapping
}

func NewEsIndexManager(logger *zap.Logger, client *elasticsearch.Client) *EsIndexManager {
	return &EsIndexManager{
		client: client,
		logger: logger,
	}
}

var (
	ioutilReadDir  = ioutil.ReadDir
	ioutilReadFile = ioutil.ReadFile
)

func (em *EsIndexManager) LoadMappings(mappingsDir string) error {
	files, err := ioutilReadDir(mappingsDir)
	if err != nil {
		return err
	}
	currentDir, err := os.Getwd()
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		documentKind := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
		filePath := path.Join(currentDir, mappingsDir, file.Name())
		versionedMappingJson, err := ioutilReadFile(filePath)

		if err != nil {
			return err
		}
		var mapping VersionedMapping

		if err := json.Unmarshal(versionedMappingJson, &mapping); err != nil {
			return err
		}

		switch documentKind {
		case ProjectDocumentKind:
			em.projectMapping = &mapping
		case OccurrenceDocumentKind:
			em.occurrenceMapping = &mapping
		case NoteDocumentKind:
			em.noteMapping = &mapping
		default:
			return fmt.Errorf("unrecognized document kind mapping: %s", documentKind)
		}
	}

	return nil
}

func (em *EsIndexManager) CreateIndex(ctx context.Context, info *IndexInfo, checkExists bool) error {
	log := em.logger.Named("CreateIndex").With(zap.String("index", info.Index))

	if checkExists {
		res, err := em.client.Indices.Exists([]string{info.Index}, em.client.Indices.Exists.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("error checking if index %s exists: %s", info.Index, err)
		}

		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNotFound {
			log.Error("error checking if index exists", zap.String("response", res.String()), zap.Int("status", res.StatusCode))

			return fmt.Errorf("unexpected status code (%d) when checking if index exists", res.StatusCode)
		}

		if !res.IsError() {
			return nil
		}
	}

	mapping, err := em.getMappingForDocumentKind(info.DocumentKind)
	if err != nil {
		return err
	}

	createIndexReq := map[string]interface{}{
		"mappings": mapping.Mappings,
	}

	if info.Alias != "" {
		createIndexReq["aliases"] = map[string]interface{}{
			info.Alias: map[string]interface{}{},
		}
	}

	payload, _ := EncodeRequest(&createIndexReq)
	res, err := em.client.Indices.Create(info.Index, em.client.Indices.Create.WithContext(ctx), em.client.Indices.Create.WithBody(payload))
	if err != nil {
		return fmt.Errorf("error creating index %s: %s", info.Index, err)
	}

	if res.IsError() {
		if res.StatusCode == http.StatusBadRequest {
			errResponse := ESErrorResponse{}
			if err := DecodeResponse(res.Body, &errResponse); err != nil {
				return fmt.Errorf("error decoding Elasticsearch error response: %s", err)
			}

			if errResponse.Error.Type == "resource_already_exists_exception" {
				log.Info("index already exists")
				return nil
			}
		}

		return fmt.Errorf("error creating index, status: %d", res.StatusCode)
	}

	log.Info("index created", zap.String("index", info.Index))

	return nil
}

func (em *EsIndexManager) getMappingForDocumentKind(documentKind string) (*VersionedMapping, error) {
	switch documentKind {
	case ProjectDocumentKind:
		return em.projectMapping, nil
	case OccurrenceDocumentKind:
		return em.occurrenceMapping, nil
	case NoteDocumentKind:
		return em.noteMapping, nil
	default:
		em.logger.Info("Unrecognized document kind mapping", zap.String("kind", documentKind))
		return nil, fmt.Errorf("no mapping found for document kind %s", documentKind)
	}
}

func (em *EsIndexManager) ProjectsIndex() string {
	indexVersion := em.projectMapping.Version
	return fmt.Sprintf("%s-%s-projects", IndexPrefix, indexVersion)
}

func (em *EsIndexManager) ProjectsAlias() string {
	return fmt.Sprintf("%s-projects", AliasPrefix)
}

func (em *EsIndexManager) OccurrencesIndex(projectId string) string {
	indexVersion := em.occurrenceMapping.Version
	return fmt.Sprintf("%s-%s-%s-occurrences", IndexPrefix, indexVersion, projectId)
}

func (em *EsIndexManager) OccurrencesAlias(projectId string) string {
	return fmt.Sprintf("%s-%s-occurrences", AliasPrefix, projectId)
}

func (em *EsIndexManager) NotesIndex(projectId string) string {
	indexVersion := em.noteMapping.Version
	return fmt.Sprintf("%s-%s-%s-notes", IndexPrefix, indexVersion, projectId)
}

func (em *EsIndexManager) NotesAlias(projectId string) string {
	return fmt.Sprintf("%s-%s-notes", AliasPrefix, projectId)
}

func (em *EsIndexManager) IncrementIndexVersion(indexName string) string {
	indexParts := parseIndexName(indexName)

	switch indexParts.DocumentKind {
	case NoteDocumentKind:
		return em.NotesIndex(indexParts.ProjectId)
	case OccurrenceDocumentKind:
		return em.OccurrencesIndex(indexParts.ProjectId)
	case ProjectDocumentKind:
		return em.ProjectsIndex()
	}

	// unversioned index
	return indexName
}

func (em *EsIndexManager) GetLatestVersionForDocumentKind(documentKind string) string {
	switch documentKind {
	case NoteDocumentKind:
		return em.noteMapping.Version
	case OccurrenceDocumentKind:
		return em.occurrenceMapping.Version
	case ProjectDocumentKind:
		return em.projectMapping.Version
	}

	return ""
}

func (em *EsIndexManager) GetAliasForIndex(indexName string) string {
	parts := parseIndexName(indexName)

	switch parts.DocumentKind {
	case NoteDocumentKind:
		return em.NotesAlias(parts.ProjectId)
	case OccurrenceDocumentKind:
		return em.OccurrencesAlias(parts.ProjectId)
	case ProjectDocumentKind:
		return em.ProjectsAlias()
	}

	return ""
}

func parseIndexName(indexName string) *IndexNameParts {
	indexParts := strings.Split(indexName, "-")
	documentKind := indexParts[len(indexParts)-1]
	nameParts := &IndexNameParts{
		DocumentKind: documentKind,
	}

	switch documentKind {
	case ProjectDocumentKind:
		nameParts.Version = indexParts[1]
	case NoteDocumentKind,
		OccurrenceDocumentKind:
		nameParts.Version = indexParts[1]
		nameParts.ProjectId = strings.Join(indexParts[2:len(indexParts)-1], "-")
	}

	return nameParts
}
