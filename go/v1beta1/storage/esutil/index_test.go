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

package esutil

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("index manager", func() {
	var (
		indexManager    *EsIndexManager
		mockEsTransport *MockEsTransport
		projectId       string
	)

	BeforeEach(func() {
		projectId = fake.LetterN(10)
		mockEsTransport = &MockEsTransport{}
		mockEsClient := &elasticsearch.Client{Transport: mockEsTransport, API: esapi.New(mockEsTransport)}

		indexManager = NewEsIndexManager(logger, mockEsClient)

		populateIndexMappings(indexManager)
	})

	Describe("LoadMappings", func() {
		var (
			actualError        error
			occurrencesMapping *VersionedMapping
			projectsMapping    *VersionedMapping
			notesMapping       *VersionedMapping
		)

		JustBeforeEach(func() {
			actualError = indexManager.LoadMappings("test")
		})

		Describe("mappings loaded successfully", func() {
			BeforeEach(func() {
				occurrencesMapping = createVersionedMapping()
				projectsMapping = createVersionedMapping()
				notesMapping = createVersionedMapping()

				ioutilReadDir = func(dirName string) ([]os.FileInfo, error) {
					return []os.FileInfo{
						&fakeFileInfo{
							name: "occurrences.json",
						},
						&fakeFileInfo{
							name: "projects.json",
						},
						&fakeFileInfo{
							name: "notes.json",
						},
						&fakeFileInfo{
							name:  fake.Word(),
							isDir: true,
						},
					}, nil
				}

				ioutilReadFile = func(fileName string) ([]byte, error) {
					if strings.Contains(fileName, "projects.json") {
						bytes, _ := json.Marshal(projectsMapping)

						return bytes, nil
					}

					if strings.Contains(fileName, "occurrences.json") {
						bytes, _ := json.Marshal(occurrencesMapping)

						return bytes, nil
					}

					if strings.Contains(fileName, "notes.json") {
						bytes, _ := json.Marshal(notesMapping)

						return bytes, nil
					}

					return nil, fmt.Errorf("unexpected file name: %s", fileName)
				}
			})

			It("should not return an error", func() {
				Expect(actualError).To(BeNil())
			})

			It("should populate the project mapping", func() {
				Expect(indexManager.projectMapping).To(Equal(projectsMapping))
			})

			It("should populate the occurrence mapping", func() {
				Expect(indexManager.occurrenceMapping).To(Equal(occurrencesMapping))
			})

			It("should populate the note mapping", func() {
				Expect(indexManager.noteMapping).To(Equal(notesMapping))
			})
		})

		Describe("mappings fail to load", func() {
			var (
				expectedError error
				fileInfo      *fakeFileInfo
			)

			BeforeEach(func() {
				expectedError = fmt.Errorf(fake.Word())
				fileInfo = &fakeFileInfo{name: fake.Word()}
				ioutilReadDir = func(dirname string) ([]os.FileInfo, error) {
					return []os.FileInfo{fileInfo}, nil
				}
			})

			Describe("an error occurs while listing the mappings directory", func() {
				BeforeEach(func() {
					expectedError = fmt.Errorf(fake.Word())
					ioutilReadDir = func(dirname string) ([]os.FileInfo, error) {
						return nil, expectedError
					}
				})

				It("should return the error", func() {
					Expect(actualError).To(MatchError(expectedError))
				})
			})

			Describe("reading a mapping file returns an error", func() {
				BeforeEach(func() {
					ioutilReadFile = func(fileName string) ([]byte, error) {
						return nil, expectedError
					}
				})

				It("should return an error", func() {
					Expect(actualError).To(MatchError(expectedError))
				})
			})

			Describe("a mapping file contains invalid json", func() {
				BeforeEach(func() {
					ioutilReadFile = func(fileName string) ([]byte, error) {
						return []byte{'}'}, nil
					}
				})

				It("should return an error", func() {
					Expect(actualError).NotTo(BeNil())
				})
			})

			Describe("a mapping file contains an unrecognized document kind", func() {
				BeforeEach(func() {
					unrecognizedMapping := createVersionedMapping()
					ioutilReadFile = func(fileName string) ([]byte, error) {
						bytes, _ := json.Marshal(unrecognizedMapping)
						return bytes, nil
					}
				})

				It("should return an error", func() {
					Expect(actualError).To(MatchError("unrecognized document kind mapping: " + fileInfo.name))
				})
			})
		})
	})

	Context("alias name functions", func() {
		Describe("ProjectsAlias", func() {
			It("should return the project alias", func() {
				Expect(indexManager.ProjectsAlias()).To(Equal(CreateIndexOrAliasName("projects")))
			})
		})

		Describe("OccurrencesAlias", func() {
			It("should construct the alias for the given project's occurrences index", func() {
				Expect(indexManager.OccurrencesAlias(projectId)).To(Equal(CreateIndexOrAliasName(projectId, "occurrences")))
			})
		})

		Describe("NotesAlias", func() {
			It("should construct the alias for the given project's notes index", func() {
				Expect(indexManager.NotesAlias(projectId)).To(Equal(CreateIndexOrAliasName(projectId, "notes")))
			})
		})
	})

	Context("index name functions", func() {
		Describe("ProjectsIndex", func() {
			It("should return the versioned projects index", func() {
				expectedIndexName := CreateIndexOrAliasName(indexManager.projectMapping.Version, "projects")

				Expect(indexManager.ProjectsIndex()).To(Equal(expectedIndexName))
			})
		})

		Describe("OccurrencesIndex", func() {
			It("should return the versioned occurrences index", func() {
				expectedIndexName := CreateIndexOrAliasName(indexManager.occurrenceMapping.Version, projectId, "occurrences")

				Expect(indexManager.OccurrencesIndex(projectId)).To(Equal(expectedIndexName))
			})
		})

		Describe("NotesIndex", func() {
			It("should return the versioned notes index", func() {
				expectedIndexName := CreateIndexOrAliasName(indexManager.noteMapping.Version, projectId, "notes")

				Expect(indexManager.NotesIndex(projectId)).To(Equal(expectedIndexName))
			})
		})
	})

	Describe("IncrementIndexVersion", func() {
		It("should return the projects index at the newest version", func() {
			expectedIndexName := CreateIndexOrAliasName(indexManager.projectMapping.Version, "projects")
			indexName := CreateIndexOrAliasName(fake.LetterN(5), "projects")

			Expect(indexManager.IncrementIndexVersion(indexName)).To(Equal(expectedIndexName))
		})

		It("should return the occurrences index at the newest version", func() {
			expectedIndexName := CreateIndexOrAliasName(indexManager.occurrenceMapping.Version, projectId, "occurrences")
			indexName := CreateIndexOrAliasName(fake.LetterN(5), projectId, "occurrences")

			Expect(indexManager.IncrementIndexVersion(indexName)).To(Equal(expectedIndexName))
		})

		It("should return the notes index at the newest version", func() {
			expectedIndexName := CreateIndexOrAliasName(indexManager.noteMapping.Version, projectId, "notes")
			indexName := CreateIndexOrAliasName(fake.LetterN(5), projectId, "notes")

			Expect(indexManager.IncrementIndexVersion(indexName)).To(Equal(expectedIndexName))
		})

		It("should return the index for unrecognized document kinds", func() {
			indexName := fake.LetterN(10)

			Expect(indexManager.IncrementIndexVersion(indexName)).To(Equal(indexName))
		})
	})

	Describe("GetLatestVersionForDocumentKind", func() {
		It("should return the version from the project mapping for the projects document kind", func() {
			expectedVersion := indexManager.projectMapping.Version
			actualVersion := indexManager.GetLatestVersionForDocumentKind(ProjectDocumentKind)

			Expect(actualVersion).To(Equal(expectedVersion))
		})

		It("should return the version from the occurrences mapping for the occurrences document kind", func() {
			expectedVersion := indexManager.occurrenceMapping.Version
			actualVersion := indexManager.GetLatestVersionForDocumentKind(OccurrenceDocumentKind)

			Expect(actualVersion).To(Equal(expectedVersion))
		})

		It("should return the version from the notes mapping for the notes document kind", func() {
			expectedVersion := indexManager.noteMapping.Version
			actualVersion := indexManager.GetLatestVersionForDocumentKind(NoteDocumentKind)

			Expect(actualVersion).To(Equal(expectedVersion))
		})

		It("should return an empty string for unrecognized document kinds", func() {
			Expect(indexManager.GetLatestVersionForDocumentKind(DocumentKind(fake.Word()))).To(BeEmpty())
		})
	})

	Describe("GetAliasForIndex", func() {
		It("should return the projects alias given a project index", func() {
			projectIndex := CreateIndexOrAliasName(fake.LetterN(5), "projects")
			expectedAlias := CreateIndexOrAliasName("projects")

			Expect(indexManager.GetAliasForIndex(projectIndex)).To(Equal(expectedAlias))
		})

		It("should return the occurrences alias given an occurrence index", func() {
			occurrencesIndex := CreateIndexOrAliasName(fake.LetterN(5), projectId, "occurrences")
			expectedAlias := CreateIndexOrAliasName(projectId, "occurrences")

			Expect(indexManager.GetAliasForIndex(occurrencesIndex)).To(Equal(expectedAlias))
		})

		It("should return the notes alias given an notes index", func() {
			notesIndex := CreateIndexOrAliasName(fake.LetterN(5), projectId, "notes")
			expectedAlias := CreateIndexOrAliasName(projectId, "notes")

			Expect(indexManager.GetAliasForIndex(notesIndex)).To(Equal(expectedAlias))
		})

		It("should return an empty string for an unrecognized document kind", func() {
			indexName := CreateIndexOrAliasName(fake.LetterN(5))

			Expect(indexManager.GetAliasForIndex(indexName)).To(BeEmpty())
		})
	})

	Describe("CreateIndex", func() {
		var (
			ctx       context.Context
			indexInfo *IndexInfo
		)

		BeforeEach(func() {
			indexInfo = randomIndexInfo(projectId)
			ctx = context.Background()
		})

		Describe("index already exists", func() {
			BeforeEach(func() {
				mockEsTransport.PreparedHttpResponses = []*http.Response{
					{
						StatusCode: http.StatusOK,
					},
				}
			})

			It("should not try to recreate the index", func() {
				err := indexManager.CreateIndex(ctx, indexInfo, true)

				Expect(err).To(BeNil())
				Expect(mockEsTransport.ReceivedHttpRequests).To(HaveLen(1))
				Expect(mockEsTransport.ReceivedHttpRequests[0].Method).To(Equal(http.MethodHead))
				Expect(mockEsTransport.ReceivedHttpRequests[0].URL.Path).To(Equal("/" + indexInfo.Index))
			})

			Describe("transport error", func() {
				var (
					actualError   error
					expectedError error
				)

				BeforeEach(func() {
					expectedError = fmt.Errorf(fake.Sentence(5))

					mockEsTransport.Actions = []TransportAction{
						func(req *http.Request) (*http.Response, error) {
							return nil, expectedError
						},
					}

					actualError = indexManager.CreateIndex(ctx, indexInfo, true)
				})

				It("should return an error", func() {
					Expect(actualError).NotTo(BeNil())
					Expect(actualError.Error()).To(ContainSubstring(expectedError.Error()))
				})
			})

			Describe("unexpected status code in response", func() {
				var actualError error

				BeforeEach(func() {
					mockEsTransport.PreparedHttpResponses[0].StatusCode = http.StatusInternalServerError

					actualError = indexManager.CreateIndex(ctx, indexInfo, true)
				})

				It("should return an error", func() {
					expectedErrorMessage := fmt.Sprintf("unexpected status code (%d) when checking if index exists", http.StatusInternalServerError)

					Expect(actualError).NotTo(BeNil())
					Expect(actualError.Error()).To(ContainSubstring(expectedErrorMessage))
				})
			})
		})

		Describe("index does not exist", func() {
			var actualError error

			BeforeEach(func() {
				mockEsTransport.PreparedHttpResponses = []*http.Response{
					{
						StatusCode: http.StatusNotFound,
					},
					{
						StatusCode: http.StatusOK,
					},
				}

				actualError = indexManager.CreateIndex(ctx, indexInfo, true)
			})

			It("should not return an error", func() {
				Expect(actualError).To(BeNil())
			})

			It("should create the index", func() {
				Expect(mockEsTransport.ReceivedHttpRequests).To(HaveLen(2))
				Expect(mockEsTransport.ReceivedHttpRequests[1].Method).To(Equal(http.MethodPut))
				Expect(mockEsTransport.ReceivedHttpRequests[1].URL.Path).To(Equal("/" + indexInfo.Index))
			})

			It("should pass the correct mappings", func() {
				expectedPayload := map[string]interface{}{
					"mappings": indexManager.occurrenceMapping.Mappings,
					"aliases": map[string]interface{}{
						indexInfo.Alias: map[string]interface{}{},
					},
				}
				actualPayload := map[string]interface{}{}

				ReadRequestBody(mockEsTransport.ReceivedHttpRequests[1], &actualPayload)

				Expect(actualPayload).To(Equal(expectedPayload))
			})
		})

		Describe("error creating index", func() {
			Describe("transport error", func() {
				var (
					expectedError error
				)

				BeforeEach(func() {
					expectedError = fmt.Errorf(fake.Sentence(5))

					mockEsTransport.Actions = []TransportAction{
						func(req *http.Request) (*http.Response, error) {
							return nil, expectedError
						},
					}
				})

				It("should return the error", func() {
					actualError := indexManager.CreateIndex(ctx, indexInfo, false)

					Expect(actualError).To(MatchError(fmt.Sprintf("error creating index %s: %s", indexInfo.Index, expectedError)))
				})
			})

			Describe("response error", func() {

				Context("bad request status code", func() {
					var errorResponse ESErrorResponse

					BeforeEach(func() {
						errorResponse := ESErrorResponse{
							Error: ESError{
								Type: fake.Word(),
							},
						}

						mockEsTransport.PreparedHttpResponses = []*http.Response{
							{
								StatusCode: http.StatusBadRequest,
							},
						}

						mockEsTransport.PreparedHttpResponses[0].Body = CreateESBody(&errorResponse)
					})

					Describe("index already exists", func() {
						BeforeEach(func() {
							errorResponse.Error.Type = "resource_already_exists_exception"

							mockEsTransport.PreparedHttpResponses[0].Body = CreateESBody(&errorResponse)
						})

						It("should not return an error", func() {
							actualError := indexManager.CreateIndex(ctx, indexInfo, false)

							Expect(actualError).To(BeNil())
						})
					})

					Describe("other error type", func() {
						It("should return an error", func() {
							actualError := indexManager.CreateIndex(ctx, indexInfo, false)

							Expect(actualError).NotTo(BeNil())
						})
					})

				})

				Describe("any other status", func() {
					BeforeEach(func() {
						mockEsTransport.PreparedHttpResponses = []*http.Response{
							{
								StatusCode: http.StatusInternalServerError,
							},
						}
					})

					It("should return an error", func() {
						actualError := indexManager.CreateIndex(ctx, indexInfo, false)

						Expect(actualError).NotTo(BeNil())
					})
				})
			})
		})
	})
})

func populateIndexMappings(indexManager *EsIndexManager) {
	indexManager.projectMapping = createVersionedMapping()
	indexManager.occurrenceMapping = createVersionedMapping()
	indexManager.noteMapping = createVersionedMapping()
}

func createVersionedMapping() *VersionedMapping {
	return &VersionedMapping{
		Mappings: map[string]interface{}{
			fake.Word(): fake.Word(),
		},
		Version: fake.LetterN(5),
	}
}

func randomIndexInfo(projectId string) *IndexInfo {
	return &IndexInfo{
		Alias:        CreateIndexOrAliasName(projectId, string(OccurrenceDocumentKind)),
		DocumentKind: OccurrenceDocumentKind,
		Index:        CreateIndexOrAliasName(fake.LetterN(5), projectId, string(OccurrenceDocumentKind)),
	}
}

type fakeFileInfo struct {
	name  string
	isDir bool
}

func (f *fakeFileInfo) Name() string {
	return f.name
}

func (f *fakeFileInfo) Size() int64 {
	return 0
}

func (f *fakeFileInfo) Mode() os.FileMode {
	return os.ModeTemporary
}

func (f *fakeFileInfo) ModTime() time.Time {
	return time.Now()
}

func (f *fakeFileInfo) IsDir() bool {
	return f.isDir
}

func (f *fakeFileInfo) Sys() interface{} {
	return nil
}