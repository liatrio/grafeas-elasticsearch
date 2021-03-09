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

package storage

import (
	"fmt"
	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	"github.com/golang/mock/gomock"
	grafeasConfig "github.com/grafeas/grafeas/go/config"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rode/grafeas-elasticsearch/go/config"
	"github.com/rode/grafeas-elasticsearch/go/mocks"
	"net/http"
)

var _ = Describe("Grafeas integration", func() {
	var (
		elasticsearchStorage    *ElasticsearchStorage
		transport               *mockEsTransport
		mockCtrl                *gomock.Controller
		filterer                *mocks.MockFilterer
		esConfig                *config.ElasticsearchConfig
		newElasticsearchStorage newElasticsearchStorageFunc
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		filterer = mocks.NewMockFilterer(mockCtrl)
		transport = &mockEsTransport{}
		esConfig = &config.ElasticsearchConfig{
			URL:     fake.URL(),
			Refresh: config.RefreshTrue,
		}
	})

	JustBeforeEach(func() {
		mockEsClient := &elasticsearch.Client{Transport: transport, API: esapi.New(transport)}

		elasticsearchStorage = NewElasticsearchStorage(logger, mockEsClient, filterer, esConfig, nil)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("creating the elasticsearch storage provider", func() {
		var (
			err                                       error
			expectedStorageType, expectedProjectIndex string
			storageConfig                             grafeasConfig.StorageConfiguration
		)

		// BeforeEach configures the happy path for this context
		// Variables configured here may be overridden in nested BeforeEach blocks
		BeforeEach(func() {
			transport.preparedHttpResponses = []*http.Response{
				{
					StatusCode: http.StatusOK,
				},
				{
					StatusCode: http.StatusOK,
				},
			}
			storageConfig = grafeasConfig.StorageConfiguration(esConfig)
			expectedProjectIndex = fmt.Sprintf("%s-%s", indexPrefix, "projects")
			expectedStorageType = "elasticsearch"

			newElasticsearchStorage = func(ec *config.ElasticsearchConfig) (*ElasticsearchStorage, error) {
				return elasticsearchStorage, nil
			}
		})

		// JustBeforeEach actually invokes the system under test
		JustBeforeEach(func() {
			registerStorageTypeProvider := ElasticsearchStorageTypeProviderCreator(newElasticsearchStorage, logger)
			_, err = registerStorageTypeProvider(expectedStorageType, &storageConfig)
		})

		It("should check if an index for projects has already been created", func() {
			Expect(transport.receivedHttpRequests[0].URL.Path).To(Equal(fmt.Sprintf("/%s", expectedProjectIndex)))
			Expect(transport.receivedHttpRequests[0].Method).To(Equal(http.MethodHead))
			Expect(err).ToNot(HaveOccurred())
		})

		When("storage configuration is not parsable", func() {
			BeforeEach(func() {
				storageConfig = grafeasConfig.StorageConfiguration("")
			})

			It("should return error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

		When("storage configuration is not valid", func() {
			BeforeEach(func() {
				esConfig.Refresh = "invalid"
				storageConfig = grafeasConfig.StorageConfiguration(esConfig)
			})

			It("should return error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

		When("provided storage type is not elasticsearch", func() {
			BeforeEach(func() {
				expectedStorageType = "fdas"
			})

			It("should return error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

		When("creating the elasticsearchStorage fails", func() {
			BeforeEach(func() {
				newElasticsearchStorage = func(elasticsearchConfig *config.ElasticsearchConfig) (*ElasticsearchStorage, error) {
					return nil, fmt.Errorf("fail")
				}
			})

			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})

		When("an index for projects does not exist", func() {
			BeforeEach(func() {
				transport.preparedHttpResponses[0].StatusCode = http.StatusNotFound
			})

			It("should create the index for projects", func() {
				Expect(transport.receivedHttpRequests).To(HaveLen(2))
				Expect(transport.receivedHttpRequests[1].URL.Path).To(Equal(fmt.Sprintf("/%s", expectedProjectIndex)))
				Expect(transport.receivedHttpRequests[1].Method).To(Equal(http.MethodPut))

				assertIndexCreateBodyHasMetadataAndStringMapping(transport.receivedHttpRequests[1].Body)
			})

			When("creating the index for projects returns errors from elasticsearch", func() {
				BeforeEach(func() {
					transport.preparedHttpResponses[1].StatusCode = http.StatusInternalServerError
				})

				It("should return an error", func() {
					Expect(err).To(HaveOccurred())
				})
			})
		})

		When("an index for projects already exists", func() {
			It("should not create an index for projects", func() {
				Expect(transport.receivedHttpRequests).To(HaveLen(1))
			})
		})

		When("checking for the existence of a project index fails", func() {
			BeforeEach(func() {
				transport.preparedHttpResponses[0].StatusCode = http.StatusInternalServerError
			})

			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
