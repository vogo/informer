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

package feed

import (
	"errors"
	"fmt"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DBFileName is the sqlite file holding feed data inside the active data directory.
const DBFileName = "feed.db"

//nolint:gochecknoglobals //ignore this.
var feedDataDB *gorm.DB

// schemaState records which columns already existed before AutoMigrate ran,
// so that one-shot backfills never overwrite values the user set later on.
type schemaState struct {
	sourceTableExisted bool
	hadEnabled         bool
}

// InitFeedDB opens the feed database inside dataDir and brings the schema up to date.
// It returns the handle so that the service layer owns data access instead of the CLI.
// Any migration failure aborts with a locatable error rather than leaving a half
// migrated schema behind.
func InitFeedDB(dataDir string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(dataDir, DBFileName)), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open feed database in %q: %w", dataDir, err)
	}

	state := inspectSchema(db)

	if err = migrateSchema(db); err != nil {
		return nil, err
	}

	if err = seedDefaultCategory(db); err != nil {
		return nil, err
	}

	if err = backfillSources(db, state); err != nil {
		return nil, err
	}

	feedDataDB = db

	return db, nil
}

func inspectSchema(db *gorm.DB) schemaState {
	migrator := db.Migrator()

	state := schemaState{sourceTableExisted: migrator.HasTable(&Source{})}
	if state.sourceTableExisted {
		state.hadEnabled = migrator.HasColumn(&Source{}, "enabled")
	}

	return state
}

func migrateSchema(db *gorm.DB) error {
	for _, model := range []any{&Category{}, &Config{}, &Source{}, &Article{}} {
		if err := db.AutoMigrate(model); err != nil {
			return fmt.Errorf("migrate feed schema %T: %w", model, err)
		}
	}

	return nil
}

// seedDefaultCategory makes sure the "未分类" category with id 1 exists, without
// touching a record the user may have renamed or re-sorted.
func seedDefaultCategory(db *gorm.DB) error {
	var existing Category

	err := db.Where("id = ?", DefaultCategoryID).First(&existing).Error
	if err == nil {
		return nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("read default category: %w", err)
	}

	if err = db.Create(&Category{ID: DefaultCategoryID, Name: DefaultCategoryName}).Error; err != nil {
		return fmt.Errorf("seed default category: %w", err)
	}

	return nil
}

// backfillSources fills the values the new columns need on upgraded databases.
// Every statement is idempotent and only touches rows that still carry no explicit value.
func backfillSources(db *gorm.DB, state schemaState) error {
	// sources without a category always point at the seeded default one.
	if err := db.Model(&Source{}).
		Where("category_id IS NULL OR category_id = 0").
		Update("category_id", DefaultCategoryID).Error; err != nil {
		return fmt.Errorf("backfill source category: %w", err)
	}

	// the enabled column is filled exactly once, on the run that adds it,
	// so a source disabled later is never switched back on.
	if state.sourceTableExisted && !state.hadEnabled {
		if err := db.Model(&Source{}).Where("1 = 1").Update("enabled", true).Error; err != nil {
			return fmt.Errorf("backfill source enabled: %w", err)
		}
	}

	// only sources without an explicit parse type get the derived value.
	if err := db.Model(&Source{}).
		Where("parse_type IS NULL OR parse_type = ''").
		Update("parse_type", gorm.Expr(
			"CASE WHEN is_json THEN ? WHEN regex IS NOT NULL AND regex != '' THEN ? ELSE ? END",
			ParseTypeJSON, ParseTypeRegex, ParseTypeFeed)).Error; err != nil {
		return fmt.Errorf("backfill source parse type: %w", err)
	}

	return nil
}
