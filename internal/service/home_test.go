/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package service_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/home"
	"github.com/vogo/informer/internal/inform"
	"github.com/vogo/informer/internal/service"
)

// informRun is what one full cycle produced, reduced to what must not depend on
// where the data directory happens to live.
type informRun struct {
	homeDir      string
	articleKeys  []string
	contentFile  string
	fileContents string
}

// runFullCycle resolves the data directory the way the CLI does, then fetches and informs.
func runFullCycle(t *testing.T, server *httptest.Server) informRun {
	t.Helper()

	dir, err := home.Init("")
	require.NoError(t, err)

	writeConfigFile(t, dir, &inform.Config{Feed: &feed.Config{
		MaxInformFeedSize: 10,
		FeedExpireDays:    3650,
		SameSiteMaxCount:  5,
		MaxFetchNum:       10,
	}})

	svc, err := service.New(dir)
	require.NoError(t, err)
	require.NoError(t, svc.CreateSource(feedSource(server)))
	require.NoError(t, svc.CreateSource(regexSource(server)))

	result, err := svc.TriggerInform("")
	require.NoError(t, err)

	page, err := svc.ListArticles(service.ArticleQuery{}, service.PageRequest{Limit: service.MaxPageLimit})
	require.NoError(t, err)

	keys := make([]string, 0, len(page.Items))
	for _, article := range page.Items {
		require.NotNil(t, article.FetchedAt, "article %q must carry a fetch time", article.Title)
		keys = append(keys, article.Title+"|"+article.URL)
	}

	contents, err := os.ReadFile(result.ContentFilePath)
	require.NoError(t, err)

	return informRun{
		homeDir:      dir,
		articleKeys:  keys,
		contentFile:  result.ContentFilePath,
		fileContents: recommendationSection(string(contents)),
	}
}

// recommendationSection cuts the article block out of the daily file, so that the
// comparison does not depend on the external daily soup service.
func recommendationSection(content string) string {
	index := strings.Index(content, "文章推荐:")
	if index < 0 {
		return ""
	}

	return content[index:]
}

// TestInformBehavesTheSameWithAndWithoutEnvHome is the regression that both
// directory resolutions produce identical fetching, article selection and daily
// file output - only the location differs.
func TestInformBehavesTheSameWithAndWithoutEnvHome(t *testing.T) {
	server := newContentServer(t)

	// without INFORMER_HOME the data lives under the user home directory.
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	require.NoError(t, os.Unsetenv(home.EnvHome))

	defaultRun := runFullCycle(t, server)
	assert.Equal(t, filepath.Join(userHome, home.DefaultDirName), defaultRun.homeDir)

	// with INFORMER_HOME the very same run happens in the configured directory.
	custom := filepath.Join(t.TempDir(), "informer-home")
	t.Setenv(home.EnvHome, custom)

	customRun := runFullCycle(t, server)
	assert.Equal(t, custom, customRun.homeDir)

	assert.Equal(t, defaultRun.articleKeys, customRun.articleKeys,
		"fetching and article selection must not depend on the data directory")
	assert.Equal(t, defaultRun.fileContents, customRun.fileContents,
		"the daily file content must not depend on the data directory")

	// each run reads and writes only inside its own active directory.
	assert.Equal(t, defaultRun.homeDir, commonRoot(defaultRun.contentFile, defaultRun.homeDir))
	assert.Equal(t, customRun.homeDir, commonRoot(customRun.contentFile, customRun.homeDir))

	for _, dir := range []string{defaultRun.homeDir, customRun.homeDir} {
		_, err := os.Stat(filepath.Join(dir, feed.DBFileName))
		require.NoError(t, err, "the database lives in %s", dir)
	}
}

// commonRoot returns root when path is inside it, and the path otherwise.
func commonRoot(path, root string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || filepath.IsAbs(rel) {
		return path
	}

	return root
}
