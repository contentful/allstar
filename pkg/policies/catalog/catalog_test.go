// Copyright 2021 Allstar Authors

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package catalog

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/v59/github"
	"github.com/ossf/allstar/pkg/config"
	"github.com/ossf/allstar/pkg/policydef"
)

var query func(context.Context, interface{}, map[string]interface{}) error

type mockClient struct{}

func (m mockClient) Query(ctx context.Context, q interface{}, v map[string]interface{}) error {
	return query(ctx, q, v)
}

func TestConfigPrecedence(t *testing.T) {
	tests := []struct {
		Name      string
		Org       OrgConfig
		OrgRepo   RepoConfig
		Repo      RepoConfig
		ExpAction string
		Exp       mergedConfig
	}{
		{
			Name: "OrgOnly",
			Org: OrgConfig{
				Action: "issue",
			},
			OrgRepo:   RepoConfig{},
			Repo:      RepoConfig{},
			ExpAction: "issue",
			Exp: mergedConfig{
				Action: "issue",
			},
		},
		{
			Name: "OrgRepoOverOrg",
			Org: OrgConfig{
				Action: "issue",
			},
			OrgRepo: RepoConfig{
				Action: github.String("log"),
			},
			Repo:      RepoConfig{},
			ExpAction: "log",
			Exp: mergedConfig{
				Action: "log",
			},
		},
		{
			Name: "RepoOverAllOrg",
			Org: OrgConfig{
				Action: "issue",
			},
			OrgRepo: RepoConfig{
				Action: github.String("log"),
			},
			Repo: RepoConfig{
				Action: github.String("email"),
			},
			ExpAction: "email",
			Exp: mergedConfig{
				Action: "email",
			},
		},
		{
			Name: "RepoDisallowed",
			Org: OrgConfig{
				OptConfig: config.OrgOptConfig{
					DisableRepoOverride: true,
				},
				Action: "issue",
			},
			OrgRepo: RepoConfig{
				Action: github.String("log"),
			},
			Repo: RepoConfig{
				Action: github.String("email"),
			},
			ExpAction: "log",
			Exp: mergedConfig{
				Action: "log",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			configFetchConfig = func(ctx context.Context, c *github.Client,
				owner, repo, path string, ol config.ConfigLevel, out interface{}) error {
				switch ol {
				case config.RepoLevel:
					rc := out.(*RepoConfig)
					*rc = test.Repo
				case config.OrgRepoLevel:
					orc := out.(*RepoConfig)
					*orc = test.OrgRepo
				case config.OrgLevel:
					oc := out.(*OrgConfig)
					*oc = test.Org
				}
				return nil
			}

			s := Catalog(true)
			ctx := context.Background()

			action := s.GetAction(ctx, nil, "", "thisrepo")
			if action != test.ExpAction {
				t.Errorf("Unexpected results. want %s, got %s", test.ExpAction, action)
			}

			oc, orc, rc := getConfig(ctx, nil, "", "thisrepo")
			mc := mergeConfig(oc, orc, rc, "thisrepo")
			if diff := cmp.Diff(&test.Exp, mc); diff != "" {
				t.Errorf("Unexpected results. (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	const notifyText = `A catalog-info.yaml file can give users information about which team is responsible for the maintenance of the repository.

To fix this, add a catalog-info.yaml file to your repository, following the official documentation.
<add-confluence-link-here>`
	tests := []struct {
		Name         string
		Org          OrgConfig
		Repo         RepoConfig
		CatalogAdded string
		cofigEnabled bool
		Exp          policydef.Result
	}{
		{
			Name:         "NotEnabled",
			Org:          OrgConfig{},
			Repo:         RepoConfig{},
			CatalogAdded: "catalog-info.yaml",
			cofigEnabled: false,
			Exp: policydef.Result{
				Enabled:    false,
				Pass:       true,
				NotifyText: "",
				Details: details{
					Enabled: true,
				},
			},
		},
		{
			Name: "Pass",
			Org: OrgConfig{
				OptConfig: config.OrgOptConfig{
					OptOutStrategy: true,
				},
			},
			Repo:         RepoConfig{},
			CatalogAdded: "catalog-info.yaml",
			cofigEnabled: true,
			Exp: policydef.Result{
				Enabled:    true,
				Pass:       true,
				NotifyText: "",
				Details: details{
					Enabled: true,
				},
			},
		},
		{
			Name: "Fail",
			Org: OrgConfig{
				OptConfig: config.OrgOptConfig{
					OptOutStrategy: true,
				},
			},
			Repo:         RepoConfig{},
			CatalogAdded: "",
			cofigEnabled: true,
			Exp: policydef.Result{
				Enabled:    true,
				Pass:       false,
				NotifyText: "catalog-info.yaml file not found.\n" + fmt.Sprint(notifyText, OrgConfig{}, RepoConfig{}),
				Details: details{
					Enabled: false,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			configFetchConfig = func(ctx context.Context, c *github.Client,
				owner, repo, path string, ol config.ConfigLevel, out interface{}) error {
				if repo == "thisrepo" && ol == config.RepoLevel {
					rc := out.(*RepoConfig)
					*rc = test.Repo
				} else if ol == config.OrgLevel {
					oc := out.(*OrgConfig)
					*oc = test.Org
				}
				return nil
			}
			query = func(ctx context.Context, q interface{}, v map[string]interface{}) error {
				qc, ok := q.(*struct {
					Repository struct {
						Object struct {
							Blob struct {
								Text string
							} `graphql:"... on Blob"`
						} `graphql:"object(expression: $expression)"`
					} `graphql:"repository(owner: $owner, name: $name)"`
				})
				if !ok {
					t.Errorf("Query() called with unexpected query structure.")
				}
				qc.Repository.Object.Blob.Text = test.CatalogAdded
				return nil
			}
			configIsEnabled = func(ctx context.Context, o config.OrgOptConfig, orc, r config.RepoOptConfig,
				c *github.Client, owner, repo string) (bool, error) {
				return test.cofigEnabled, nil
			}
			res, err := check(context.Background(), nil, mockClient{}, "", "thisrepo")
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			c := cmp.Comparer(func(x, y string) bool { return trunc(x, 40) == trunc(y, 40) })
			if diff := cmp.Diff(&test.Exp, res, c); diff != "" {
				t.Errorf("Unexpected results. (-want +got):\n%s", diff)
			}
		})
	}
}

func trunc(s string, n int) string {
	if n >= len(s) {
		return s
	}
	return s[:n]
}
