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

package feed_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vogo/informer/internal/feed"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// legacySource mirrors the Source schema as it existed before this change,
// so that an upgrade can be exercised against a realistic old database.
type legacySource struct {
	ID            int64 `gorm:"primarykey;AUTO_INCREMENT"`
	Title         string
	URL           string
	CURL          string
	Weight        int64
	MaxFetchNum   int
	Regex         string
	TitleExp      string
	URLExp        string
	Redirect      bool
	Sort          bool
	IsJSON        bool
	JsonTitlePath string
	JsonURLPath   string
	Status        int
	ErrorInfo     string
}

func (legacySource) TableName() string { return "sources" }

// legacyArticle mirrors the Article schema as it existed before this change.
type legacyArticle struct {
	ID        int64  `gorm:"primarykey;AUTO_INCREMENT"`
	URL       string `gorm:"index"`
	Title     string
	Timestamp int64
	Weight    int64
	Informed  bool `gorm:"index"`
	Score     int64
	SourceID  int64
}

func (legacyArticle) TableName() string { return "articles" }

// openLegacyDB creates a database carrying only the old schema and returns its directory.
func openLegacyDB(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, feed.DBFileName)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&legacySource{}, &legacyArticle{}))

	require.NoError(t, db.Create(&[]legacySource{
		{ID: 1, Title: "plain feed", URL: "https://example.com/atom.xml"},
		{ID: 2, Title: "regex site", URL: "https://example.com/", Regex: `<a href="([^"]+)">([^<]+)</a>`},
		{ID: 3, Title: "json api", URL: "https://example.com/api", IsJSON: true},
	}).Error)

	require.NoError(t, db.Create(&[]legacyArticle{
		{ID: 1, URL: "https://example.com/a", Title: "already sent", Informed: true},
		{ID: 2, URL: "https://example.com/b", Title: "still pending", Informed: false},
	}).Error)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	return dir
}

func TestInitFeedDBOnNewDatabase(t *testing.T) {
	db, err := feed.InitFeedDB(t.TempDir())
	require.NoError(t, err)

	migrator := db.Migrator()
	for _, column := range []string{"parse_type", "category_id", "enabled"} {
		assert.True(t, migrator.HasColumn(&feed.Source{}, column), "sources.%s must exist", column)
	}

	for _, column := range []string{"fetched_at", "informed_at"} {
		assert.True(t, migrator.HasColumn(&feed.Article{}, column), "articles.%s must exist", column)
	}

	assert.True(t, migrator.HasTable(&feed.Category{}))

	var categories []*feed.Category
	require.NoError(t, db.Order("id").Find(&categories).Error)
	require.Len(t, categories, 1, "a new database holds exactly the seeded category")
	assert.Equal(t, int64(feed.DefaultCategoryID), categories[0].ID)
	assert.Equal(t, feed.DefaultCategoryName, categories[0].Name)
}

func TestInitFeedDBNewSourceDefaults(t *testing.T) {
	db, err := feed.InitFeedDB(t.TempDir())
	require.NoError(t, err)

	source := &feed.Source{
		Title:      "new",
		URL:        "https://example.com/feed",
		CategoryID: feed.DefaultCategoryID,
		Enabled:    true,
		ParseType:  feed.ParseTypeFeed,
	}
	require.NoError(t, db.Create(source).Error)

	var stored feed.Source
	require.NoError(t, db.First(&stored, source.ID).Error)
	assert.Equal(t, int64(feed.DefaultCategoryID), stored.CategoryID)
	assert.True(t, stored.Enabled)
	assert.Equal(t, feed.ParseTypeFeed, stored.ParseType)
}

func TestInitFeedDBUpgradesLegacyDatabase(t *testing.T) {
	dir := openLegacyDB(t)

	db, err := feed.InitFeedDB(dir)
	require.NoError(t, err, "AutoMigrate must succeed on an old database")

	var sources []*feed.Source
	require.NoError(t, db.Order("id").Find(&sources).Error)
	require.Len(t, sources, 3)

	wantParseType := map[int64]string{
		1: feed.ParseTypeFeed,
		2: feed.ParseTypeRegex,
		3: feed.ParseTypeJSON,
	}

	for _, source := range sources {
		assert.Equal(t, wantParseType[source.ID], source.ParseType, "source %d parse type", source.ID)
		assert.Equal(t, int64(feed.DefaultCategoryID), source.CategoryID, "source %d category", source.ID)
		assert.True(t, source.Enabled, "source %d must be enabled after the upgrade", source.ID)
	}
}

// TestInitFeedDBKeepsHistoricalArticleTimestampsEmpty pins the honesty rule:
// historical articles get no invented fetch or inform time, not even the ones
// already marked as informed.
func TestInitFeedDBKeepsHistoricalArticleTimestampsEmpty(t *testing.T) {
	dir := openLegacyDB(t)

	db, err := feed.InitFeedDB(dir)
	require.NoError(t, err)

	var articles []*feed.Article
	require.NoError(t, db.Order("id").Find(&articles).Error)
	require.Len(t, articles, 2)

	assert.True(t, articles[0].Informed, "the existing informed flag is preserved")
	assert.False(t, articles[1].Informed)

	for _, article := range articles {
		assert.Nil(t, article.FetchedAt, "article %d must keep an empty fetch time", article.ID)
		assert.Nil(t, article.InformedAt, "article %d must keep an empty inform time", article.ID)
	}

	var withTimestamps int64
	require.NoError(t, db.Model(&feed.Article{}).
		Where("informed_at IS NOT NULL OR fetched_at IS NOT NULL").Count(&withTimestamps).Error)
	assert.Zero(t, withTimestamps, "no timestamp may be invented by the migration")
}

// TestInitFeedDBIsIdempotent proves a second initialisation never reverts a manual change.
func TestInitFeedDBIsIdempotent(t *testing.T) {
	dir := openLegacyDB(t)

	db, err := feed.InitFeedDB(dir)
	require.NoError(t, err)

	other := &feed.Category{Name: "技术", Sort: 5}
	require.NoError(t, db.Create(other).Error)

	// the user disables a source, moves it and pins its parse type by hand.
	require.NoError(t, db.Model(&feed.Source{}).Where("id = ?", 1).
		Updates(map[string]any{"enabled": false, "category_id": other.ID, "parse_type": feed.ParseTypeJSON}).Error)

	// and renames the seeded category.
	require.NoError(t, db.Model(&feed.Category{}).Where("id = ?", feed.DefaultCategoryID).
		Update("name", "默认分组").Error)

	for range 2 {
		db, err = feed.InitFeedDB(dir)
		require.NoError(t, err)
	}

	var source feed.Source
	require.NoError(t, db.First(&source, 1).Error)
	assert.False(t, source.Enabled, "a source disabled by the user stays disabled")
	assert.Equal(t, other.ID, source.CategoryID, "a manual category assignment is kept")
	assert.Equal(t, feed.ParseTypeJSON, source.ParseType, "an explicit parse type is kept")

	var seeded feed.Category
	require.NoError(t, db.First(&seeded, feed.DefaultCategoryID).Error)
	assert.Equal(t, "默认分组", seeded.Name, "the seed never overwrites a renamed category")

	var defaultCount int64
	require.NoError(t, db.Model(&feed.Category{}).Where("id = ?", feed.DefaultCategoryID).Count(&defaultCount).Error)
	assert.Equal(t, int64(1), defaultCount, "the default category exists exactly once")
}

func TestInitFeedDBFailsOnMissingDirectory(t *testing.T) {
	_, err := feed.InitFeedDB(filepath.Join(t.TempDir(), "does", "not", "exist"))
	require.Error(t, err)
}
