package radarr

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/cloudbox/autoscan"
)

func TestHandler(t *testing.T) {
	type Given struct {
		Config  Config
		Fixture string
	}

	type Expected struct {
		Scans      []autoscan.Scan
		StatusCode int
	}

	type Test struct {
		Name     string
		Given    Given
		Expected Expected
	}

	standardConfig := Config{
		Name:     "radarr",
		Priority: 5,
		Rewrite: []autoscan.Rewrite{{
			From: "/Movies/*",
			To:   "/mnt/unionfs/Media/Movies/$1",
		}},
	}

	currentTime := time.Now()
	now = func() time.Time {
		return currentTime
	}

	testCases := []Test{
		{
			"Download Event",
			Given{
				Config:  standardConfig,
				Fixture: "testdata/interstellar.json",
			},
			Expected{
				StatusCode: 200,
				Scans: []autoscan.Scan{
					{
						Folder:       "/mnt/unionfs/Media/Movies/Interstellar (2014)",
						RelativePath: "Interstellar.2014.UHD.BluRay.2160p.REMUX.mkv",
						Priority:     5,
						Time:         currentTime.Unix(),
					},
				},
			},
		},
		{
			"MovieFileDelete Event",
			Given{
				Config:  standardConfig,
				Fixture: "testdata/movie_file_delete.json",
			},
			Expected{
				StatusCode: 200,
				Scans: []autoscan.Scan{
					{
						Folder:       "/mnt/unionfs/Media/Movies/Tenet (2020)",
						RelativePath: "Tenet.2020.mkv",
						Priority:     5,
						Time:         currentTime.Unix(),
					},
				},
			},
		},
		{
			"MovieDelete Event",
			Given{
				Config:  standardConfig,
				Fixture: "testdata/movie_delete.json",
			},
			Expected{
				StatusCode: 200,
				Scans: []autoscan.Scan{
					{
						Folder:   "/mnt/unionfs/Media/Movies/Wonder Woman 1984 (2020)",
						Priority: 5,
						Time:     currentTime.Unix(),
					},
				},
			},
		},
		{
			"Rename Event",
			Given{
				Config:  standardConfig,
				Fixture: "testdata/rename.json",
			},
			Expected{
				StatusCode: 200,
				Scans: []autoscan.Scan{
					{
						Folder:   "/mnt/unionfs/Media/Movies/Deadpool (2016)",
						Priority: 5,
						Time:     currentTime.Unix(),
					},
				},
			},
		},
		{
			"Returns bad request on invalid JSON",
			Given{
				Config:  standardConfig,
				Fixture: "testdata/invalid.json",
			},
			Expected{
				StatusCode: 400,
			},
		},
		{
			"Returns 200 on Test event without emitting a scan",
			Given{
				Config:  standardConfig,
				Fixture: "testdata/test.json",
			},
			Expected{
				StatusCode: 200,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			callback := func(scans ...autoscan.Scan) error {
				if !reflect.DeepEqual(tc.Expected.Scans, scans) {
					t.Log(scans)
					t.Log(tc.Expected.Scans)
					t.Error("Scans do not equal")
					return errors.New("Scans do not equal")
				}

				return nil
			}

			trigger, err := New(tc.Given.Config)
			if err != nil {
				t.Fatalf("Could not create Radarr Trigger: %v", err)
			}

			server := httptest.NewServer(trigger(callback))
			defer server.Close()

			request, err := os.Open(tc.Given.Fixture)
			if err != nil {
				t.Fatalf("Could not open the fixture: %s", tc.Given.Fixture)
			}

			res, err := http.Post(server.URL, "application/json", request)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			defer func() { _ = res.Body.Close() }()
			if res.StatusCode != tc.Expected.StatusCode {
				t.Errorf("Status codes do not match: %d vs %d", res.StatusCode, tc.Expected.StatusCode)
			}
		})
	}
}
