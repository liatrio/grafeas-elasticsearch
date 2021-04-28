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

package v1beta1_test

import (
	"fmt"
	"github.com/grafeas/grafeas/proto/v1beta1/attestation_go_proto"
	"github.com/grafeas/grafeas/proto/v1beta1/build_go_proto"
	"github.com/grafeas/grafeas/proto/v1beta1/common_go_proto"
	"github.com/grafeas/grafeas/proto/v1beta1/grafeas_go_proto"
	"github.com/grafeas/grafeas/proto/v1beta1/package_go_proto"
	"github.com/grafeas/grafeas/proto/v1beta1/vulnerability_go_proto"
	. "github.com/onsi/gomega"
	"github.com/rode/grafeas-elasticsearch/test/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"testing"
)

func TestNote(t *testing.T) {

	Expect := util.NewExpect(t)
	s := util.NewSetup()

	// setup project for occurrences
	projectName := util.RandomProjectName()

	_, err := util.CreateProject(s, projectName)
	Expect(err).ToNot(HaveOccurred())

	t.Run("creating a note", func(t *testing.T) {
		// generate note ID (note IDs are provided by client, not generated by server)
		noteId := fake.UUID()

		t.Run("should be successful", func(t *testing.T) {
			expectedNote, err := s.Gc.CreateNote(s.Ctx, &grafeas_go_proto.CreateNoteRequest{
				Parent: projectName,
				NoteId: noteId,
				Note:   createFakeBuildNote(),
			})
			Expect(err).ToNot(HaveOccurred())

			actualNote, err := s.Gc.GetNote(s.Ctx, &grafeas_go_proto.GetNoteRequest{Name: expectedNote.GetName()})
			Expect(err).ToNot(HaveOccurred())

			Expect(actualNote).To(Equal(expectedNote))
		})
		t.Run("should return an error if the project doesn't already exist", func(t *testing.T) {
			invalidProjectName := util.RandomProjectName()
			_, err := s.Gc.CreateNote(s.Ctx, &grafeas_go_proto.CreateNoteRequest{
				Parent: invalidProjectName,
				NoteId: noteId,
				Note:   createFakeBuildNote(),
			})
			Expect(err).To(HaveOccurred())
		})
	})

	t.Run("batch creating notes", func(t *testing.T) {
		noteId1 := fake.UUID()
		noteId2 := fake.UUID()

		batch, err := s.Gc.BatchCreateNotes(s.Ctx, &grafeas_go_proto.BatchCreateNotesRequest{
			Parent: projectName,
			Notes: map[string]*grafeas_go_proto.Note{
				noteId1: createFakeBuildNote(),
				noteId2: createFakeVulnerabilityNote(),
			},
		})

		t.Run("should be successful", func(t *testing.T) {
			Expect(err).ToNot(HaveOccurred())

			for _, o := range batch.Notes {
				_, err = s.Gc.GetNote(s.Ctx, &grafeas_go_proto.GetNoteRequest{Name: o.GetName()})
				Expect(err).ToNot(HaveOccurred())
			}
		})

		t.Run("should not create notes with duplicate IDs", func(t *testing.T) {
			noteId3 := fake.UUID()

			// this will return an error, but one of the notes should have still been created
			_, err := s.Gc.BatchCreateNotes(s.Ctx, &grafeas_go_proto.BatchCreateNotesRequest{
				Parent: projectName,
				Notes: map[string]*grafeas_go_proto.Note{
					noteId1: createFakeBuildNote(),
					noteId3: createFakeVulnerabilityNote(),
				},
			})
			Expect(err).To(HaveOccurred())

			_, err = s.Gc.GetNote(s.Ctx, &grafeas_go_proto.GetNoteRequest{
				Name: fmt.Sprintf("%s/notes/%s", projectName, noteId3),
			})
			Expect(err).ToNot(HaveOccurred())
		})

		t.Run("should return an error if the project doesn't already exist", func(t *testing.T) {
			invalidProjectName := util.RandomProjectName()
			_, err := s.Gc.BatchCreateNotes(s.Ctx, &grafeas_go_proto.BatchCreateNotesRequest{
				Parent: invalidProjectName,
				Notes: map[string]*grafeas_go_proto.Note{
					noteId1: createFakeBuildNote(),
				}})
			Expect(err).To(HaveOccurred())
		})
	})

	t.Run("listing notes", func(t *testing.T) {
		// setup project specifically for listing notes
		listProjectName := util.RandomProjectName()

		_, err := util.CreateProject(s, listProjectName)
		Expect(err).ToNot(HaveOccurred())

		// creating different notes to test against
		buildNote := createFakeBuildNote()
		vulnerabilityNote := createFakeVulnerabilityNote()
		attestationNote := createFakeAttestationNote()

		secondBuildNote := createFakeBuildNote()
		secondVulnerabilityNote := createFakeVulnerabilityNote()
		secondAttestationNote := createFakeAttestationNote()

		// ensure notes have something in common to filter against
		buildNote.ShortDescription = vulnerabilityNote.ShortDescription
		vulnerabilityNote.LongDescription = attestationNote.LongDescription

		// create
		batch, err := s.Gc.BatchCreateNotes(s.Ctx, &grafeas_go_proto.BatchCreateNotesRequest{
			Parent: listProjectName,
			Notes: map[string]*grafeas_go_proto.Note{
				"build1": buildNote,
				"build2": secondBuildNote,
				"vuln1":  vulnerabilityNote,
				"vuln2":  secondVulnerabilityNote,
				"att1":   attestationNote,
				"att2":   secondAttestationNote,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		// reassign pointer values for test notes, since the created notes will have a new `Name` field that
		// will need to be included in our assertions
		for _, note := range batch.Notes {
			switch strings.Split(note.Name, "/")[3] {
			case "build1":
				buildNote = note
			case "build2":
				secondBuildNote = note
			case "vuln1":
				vulnerabilityNote = note
			case "vuln2":
				secondVulnerabilityNote = note
			case "att1":
				attestationNote = note
			case "att2":
				secondAttestationNote = note
			}
		}

		t.Run("should be successful", func(t *testing.T) {
			res, err := s.Gc.ListNotes(s.Ctx, &grafeas_go_proto.ListNotesRequest{
				Parent: listProjectName,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Notes).To(HaveLen(6))
		})

		t.Run("filters", func(t *testing.T) {
			for _, tc := range []struct {
				name, filter string
				expected     []*grafeas_go_proto.Note
				expectError  bool
			}{
				{
					name:   "match build type",
					filter: `kind=="BUILD"`,
					expected: []*grafeas_go_proto.Note{
						buildNote,
						secondBuildNote,
					},
				},
				{
					name:   "match vuln type",
					filter: `kind=="VULNERABILITY"`,
					expected: []*grafeas_go_proto.Note{
						vulnerabilityNote,
						secondVulnerabilityNote,
					},
				},
				{
					name:   "match attestation type",
					filter: `kind=="ATTESTATION"`,
					expected: []*grafeas_go_proto.Note{
						attestationNote,
						secondAttestationNote,
					},
				},
				{
					name:   "match one vuln note",
					filter: fmt.Sprintf(`kind=="VULNERABILITY" && shortDescription != "%s"`, vulnerabilityNote.ShortDescription),
					expected: []*grafeas_go_proto.Note{
						secondVulnerabilityNote,
					},
				},
			} {
				// ensure parallel tests are run with correct test case
				tc := tc

				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					res, err := s.Gc.ListNotes(s.Ctx, &grafeas_go_proto.ListNotesRequest{
						Parent: listProjectName,
						Filter: tc.filter,
					})
					if tc.expectError {
						Expect(err).To(HaveOccurred())
					} else {
						Expect(err).ToNot(HaveOccurred())
						Expect(res.Notes).To(HaveLen(len(tc.expected)))
						Expect(tc.expected).To(ConsistOf(res.Notes))
					}
				})
			}
		})

		t.Run("should use pagination", func(t *testing.T) {
			var (
				foundNotes []*grafeas_go_proto.Note
				pageToken  string // start as empty by default, will be updated with each request
			)

			// we'll use pagination to list notes three times
			for i := 0; i < 3; i++ {
				pageSize := fake.Number(1, 2)
				res, err := s.Gc.ListNotes(s.Ctx, &grafeas_go_proto.ListNotesRequest{
					Parent:    listProjectName,
					PageSize:  int32(pageSize),
					PageToken: pageToken,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(res.Notes).To(HaveLen(pageSize))

				isLastPage := len(res.Notes) + len(foundNotes) == len(batch.Notes)
				if isLastPage {
					Expect(res.NextPageToken).To(BeEmpty())
				} else {
					Expect(res.NextPageToken).ToNot(BeEmpty())
				}


				// ensure we have not received these notes already
				for _, o := range res.Notes {
					Expect(o).ToNot(BeElementOf(foundNotes))
				}

				// setup for next run
				pageToken = res.NextPageToken
				foundNotes = append(foundNotes, res.Notes...)
			}
		})
	})

	t.Run("deleting a note", func(t *testing.T) {
		noteId := fake.UUID()

		n, err := s.Gc.CreateNote(s.Ctx, &grafeas_go_proto.CreateNoteRequest{
			Parent: projectName,
			NoteId: noteId,
			Note:   createFakeBuildNote(),
		})
		Expect(err).ToNot(HaveOccurred())

		// Currently Grafeas returns an error even on successful delete.
		// This makes testing delete scenarios awkward.
		// For now we ignore response on delete, and check for error on a subsequent lookup, assuming it won't be found.
		//
		// TODO: Once a new version of Grafeas is released that contains this fix:
		//  https://github.com/grafeas/grafeas/pull/456
		//  This should be updated to actually review delete results

		_, _ = s.Gc.DeleteNote(s.Ctx, &grafeas_go_proto.DeleteNoteRequest{
			Name: n.GetName(),
		})

		_, err = s.Gc.GetNote(s.Ctx, &grafeas_go_proto.GetNoteRequest{
			Name: n.GetName(),
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.NotFound))
	})
}

func createFakeBuildNote() *grafeas_go_proto.Note {
	return &grafeas_go_proto.Note{
		Name:             fake.LetterN(10),
		ShortDescription: fake.LoremIpsumSentence(fake.Number(5, 10)),
		LongDescription:  fake.LoremIpsumSentence(fake.Number(5, 10)),
		Kind:             common_go_proto.NoteKind_BUILD,
		Type: &grafeas_go_proto.Note_Build{
			Build: &build_go_proto.Build{
				BuilderVersion: fake.LetterN(10),
				Signature: &build_go_proto.BuildSignature{
					PublicKey: fake.LetterN(10),
					KeyId:     fake.LetterN(10),
					Signature: []byte(fake.LetterN(10)),
				},
			},
		},
	}
}

func createFakeVulnerabilityNote() *grafeas_go_proto.Note {
	return &grafeas_go_proto.Note{
		Name:             fake.LetterN(10),
		ShortDescription: fake.LoremIpsumSentence(fake.Number(5, 10)),
		LongDescription:  fake.LoremIpsumSentence(fake.Number(5, 10)),
		Kind:             common_go_proto.NoteKind_VULNERABILITY,
		Type: &grafeas_go_proto.Note_Vulnerability{
			Vulnerability: &vulnerability_go_proto.Vulnerability{
				Details: []*vulnerability_go_proto.Vulnerability_Detail{
					{
						Package: fake.LetterN(10),
						CpeUri:  fake.LetterN(10),
						MinAffectedVersion: &package_go_proto.Version{
							Name: fake.AppVersion(),
							Kind: package_go_proto.Version_NORMAL,
						},
					},
				},
			},
		},
	}
}

func createFakeAttestationNote() *grafeas_go_proto.Note {
	return &grafeas_go_proto.Note{
		Name:             fake.LetterN(10),
		ShortDescription: fake.LoremIpsumSentence(fake.Number(5, 10)),
		LongDescription:  fake.LoremIpsumSentence(fake.Number(5, 10)),
		Kind:             common_go_proto.NoteKind_ATTESTATION,
		Type: &grafeas_go_proto.Note_AttestationAuthority{
			AttestationAuthority: &attestation_go_proto.Authority{
				Hint: &attestation_go_proto.Authority_Hint{
					HumanReadableName: fake.LetterN(10),
				},
			},
		},
	}
}
