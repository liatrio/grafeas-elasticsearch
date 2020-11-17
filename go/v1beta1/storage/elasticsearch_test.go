package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	pb "github.com/grafeas/grafeas/proto/v1beta1/grafeas_go_proto"
	prpb "github.com/grafeas/grafeas/proto/v1beta1/project_go_proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("elasticsearch storage", func() {
	var (
		elasticsearchStorage *ElasticsearchStorage
		transport            *mockEsTransport
		pID                  string
		ctx                  context.Context
		project              *prpb.Project
		expectedProject      *prpb.Project
		err                  error
	)

	BeforeEach(func() {
		transport = &mockEsTransport{}
		transport.expectedError = nil
		mockEsClient := &elasticsearch.Client{Transport: transport, API: esapi.New(transport)}

		pID = "rode"
		ctx = context.Background()
		project = &prpb.Project{Name: "projects/rode"}

		elasticsearchStorage = NewElasticsearchStore(mockEsClient, logger)
	})

	Context("Creating a new Grafeas project", func() {
		When("elasticsearch successfully creates a new index", func() {
			BeforeEach(func() {
				transport.preparedPerformResponse = &http.Response{
					StatusCode: 200,
				}

				expectedProject, err = elasticsearchStorage.CreateProject(ctx, pID, project)
			})

			It("should have performed a PUT Request", func() {
				Expect(transport.receivedPerformRequest.Method).To(Equal("PUT"))
			})

			It("should have created an index at a path matching the PID", func() {
				Expect(transport.receivedPerformRequest.URL.Path).To(Equal(fmt.Sprintf("/%s", pID)))
			})

			It("should return a new Grafeas project", func() {
				Expect(expectedProject.Name).To(Equal("projects/rode"))
			})

			It("should return without an error", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("elasticsearch unsuccessfully creates a new index", func() {
			BeforeEach(func() {
				transport.expectedError = errors.New("failed to create new index")
				_, err = elasticsearchStorage.CreateProject(ctx, pID, project)
			})

			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("Creating a new Grafeas occurrence", func() {
		var (
			uID                string
			newOccurrence      *pb.Occurrence
			expectedOccurrence *pb.Occurrence
		)

		BeforeEach(func() {
			uID = "sonarqubeMetric"
			newOccurrence = &pb.Occurrence{}
		})

		When("elasticsearch creates a new document", func() {
			BeforeEach(func() {
				transport.preparedPerformResponse = &http.Response{
					StatusCode: 201,
				}

				expectedOccurrence, err = elasticsearchStorage.CreateOccurrence(ctx, pID, uID, newOccurrence)
			})

			It("should perform a PUT request", func() {
				Expect(transport.receivedPerformRequest.Method).To(Equal("PUT"))
			})

			It("should have created an index at a path matching the PID", func() {
				Expect(transport.receivedPerformRequest.URL.Path).To(Equal(fmt.Sprintf("/%s/_doc", pID)))
			})

			It("should return a Grafeas occurrence", func() {
				Expect(expectedOccurrence).To(Equal(newOccurrence))
			})

			It("should return without an error", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("elasticsearch fails to create a new document", func() {
			BeforeEach(func() {
				transport.expectedError = errors.New("failed to create new document")
				_, err = elasticsearchStorage.CreateProject(ctx, pID, project)
			})

			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("Creating a batch of Grafeas occurrences", func() {
		var ()

		BeforeEach(func() {

		})

		When("elastic search successfully creates new documents", func() {
			BeforeEach(func() {

			})

			It("", func() {

			})
		})

		When("elastic search fails to create new documents", func() {
			BeforeEach(func() {

			})

			It("", func() {

			})
		})
	})

	Context("Deleting a Grafeas occurrence", func() {
		var ()

		BeforeEach(func() {

		})

		When("elasticsearch successfully removes a document", func() {
			BeforeEach(func() {

			})

			It("", func() {

			})
		})

		When("elasticsearch fails to remove a document", func() {
			BeforeEach(func() {

			})

			It("", func() {

			})
		})
	})
})
