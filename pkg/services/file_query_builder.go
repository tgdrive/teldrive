package services

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/WinterYukky/gorm-extra-clause-plugin/exclause"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/mapper"
	"github.com/tgdrive/teldrive/pkg/models"

	"gorm.io/gorm"
)

type fileQueryBuilder struct {
	db *gorm.DB
}

type fileResponse struct {
	models.File
	Total int
}

var selectedFields = []string{"id", "name", "type", "mime_type", "category", "channel_id", "encrypted", "size", "parent_id", "updated_at"}

func (afb *fileQueryBuilder) execute(filesQuery *api.FilesListParams, userId int64) (*api.FileList, error) {
	query := afb.db.Where("user_id = ?", userId).Where("status = ?", filesQuery.Status.Value)
	if filesQuery.Operation.Value == api.FileQueryOperationList {
		query = afb.applyListFilters(query, filesQuery, userId)
	} else if filesQuery.Operation.Value == api.FileQueryOperationFind {
		query = afb.applyFindFilters(query, filesQuery, userId)

	}
	query = afb.buildFileQuery(query, filesQuery, userId)
	res := []fileResponse{}
	if err := query.Scan(&res).Error; err != nil {
		if strings.Contains(err.Error(), "file not found") {
			return nil, &apiError{err: errors.New("invalid path"), code: 404}
		}
		return nil, &apiError{err: err}
	}
	count := 0

	if len(res) > 0 {
		count = res[0].Total
	}

	files := utils.Map(res, func(item fileResponse) api.File { return *mapper.ToFileOut(item.File) })

	return &api.FileList{Items: files,
		Meta: api.FileListMeta{Count: count,
			TotalPages:  int(math.Ceil(float64(count) / float64(filesQuery.Limit.Value))),
			CurrentPage: filesQuery.Page.Value}}, nil
}

func (afb *fileQueryBuilder) applyListFilters(query *gorm.DB, filesQuery *api.FilesListParams, userId int64) *gorm.DB {
	if filesQuery.Path.Value != "" && filesQuery.ParentId.Value == "" {
		query = query.Where("parent_id in (SELECT id FROM teldrive.get_file_from_path(?, ?, ?))", filesQuery.Path.Value, userId, true)
	}
	if filesQuery.ParentId.Value != "" {
		query = query.Where("parent_id = ?", filesQuery.ParentId.Value)
	}
	return query
}

func (afb *fileQueryBuilder) applyFindFilters(query *gorm.DB, filesQuery *api.FilesListParams, userId int64) *gorm.DB {
	if filesQuery.DeepSearch.Value && filesQuery.Query.Value != "" && filesQuery.Path.Value != "" {
		query = query.Where("files.id in (select id  from subdirs)")
	}
	if filesQuery.UpdatedAt.Value != "" {
		query, _ = afb.applyDateFilters(query, filesQuery.UpdatedAt.Value)
	}

	if filesQuery.Query.Value != "" {
		query = afb.applySearchQuery(query, filesQuery)
	}

	query = afb.applyCategoryFilter(query, filesQuery.Category)

	query = afb.applyFileSpecificFilters(query, filesQuery, userId)

	return query
}

func (afb *fileQueryBuilder) applyFileSpecificFilters(query *gorm.DB, filesQuery *api.FilesListParams, userId int64) *gorm.DB {
	if filesQuery.Name.Value != "" {
		query = query.Where("name = ?", filesQuery.Name.Value)
	}

	if filesQuery.ParentId.Value != "" {
		if filesQuery.ParentId.Value == "nil" {
			query = query.Where("parent_id is NULL")
		} else {
			query = query.Where("parent_id = ?", filesQuery.ParentId.Value)
		}

	}

	if filesQuery.ParentId.Value == "" && filesQuery.Path.Value != "" && filesQuery.Query.Value == "" {
		query = query.Where("parent_id in (SELECT id FROM teldrive.get_file_from_path(?, ?, ?))",
			filesQuery.Path.Value, userId, true)
	}

	if filesQuery.Type.Value != "" {
		query = query.Where("type = ?", filesQuery.Type.Value)
	}

	if filesQuery.Shared.Value {
		query = query.Where("id in (SELECT file_id FROM teldrive.file_shares where user_id = ?)", userId)
	}

	return query
}

func (afb *fileQueryBuilder) applyDateFilters(query *gorm.DB, dateFilters string) (*gorm.DB, error) {
	dateFiltersArr := strings.Split(dateFilters, ",")
	for _, dateFilter := range dateFiltersArr {
		query = afb.applySingleDateFilter(query, dateFilter)
	}
	return query, nil
}

func (afb *fileQueryBuilder) applySingleDateFilter(query *gorm.DB, dateFilter string) *gorm.DB {
	parts := strings.Split(dateFilter, ":")
	if len(parts) != 2 {
		return query
	}
	op, date := parts[0], parts[1]
	t, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return query
	}

	formattedDate := t.Format(time.RFC3339)
	switch op {
	case "gte":
		query = query.Where("updated_at >= ?", formattedDate)
	case "lte":
		query = query.Where("updated_at <= ?", formattedDate)
	case "eq":
		query = query.Where("updated_at = ?", formattedDate)
	case "gt":
		query = query.Where("updated_at > ?", formattedDate)
	case "lt":
		query = query.Where("updated_at < ?", formattedDate)
	}
	return query
}

func (afb *fileQueryBuilder) applySearchQuery(query *gorm.DB, filesQuery *api.FilesListParams) *gorm.DB {
	if filesQuery.SearchType.Value == api.FileQuerySearchTypeText {
		query = query.Where("name &@~ lower(regexp_replace(?, '[^[:alnum:]\\s]', ' ', 'g'))", filesQuery.Query.Value)
	} else if filesQuery.SearchType.Value == api.FileQuerySearchTypeRegex {
		query = query.Where("name &~ ?", filesQuery.Query.Value)
	}
	return query
}

func (afb *fileQueryBuilder) applyCategoryFilter(query *gorm.DB, categories []api.Category) *gorm.DB {
	if len(categories) == 0 {
		return query
	}
	var filterQuery *gorm.DB
	if categories[0] == "folder" {
		filterQuery = afb.db.Where("type = ?", categories[0])
	} else {
		filterQuery = afb.db.Where("category = ?", categories[0])
	}

	if len(categories) > 1 {
		for _, category := range categories[1:] {
			if category == "folder" {
				filterQuery = filterQuery.Or("type = ?", category)
			} else {
				filterQuery = filterQuery.Or("category = ?", category)
			}
		}
	}
	return query.Where(filterQuery)
}

func (afb *fileQueryBuilder) buildFileQuery(query *gorm.DB, filesQuery *api.FilesListParams, userId int64) *gorm.DB {
	orderField := utils.CamelToSnake(string(filesQuery.Sort.Value))
	op := getOrderOperation(filesQuery)

	return afb.buildSubqueryCTE(query, filesQuery, userId).Clauses(exclause.NewWith("ranked_scores", afb.db.Model(&models.File{}).Select(orderField, "count(*) OVER () as total",
		fmt.Sprintf("ROW_NUMBER() OVER (ORDER BY %s %s) AS rank", orderField, strings.ToUpper(string(filesQuery.Order.Value)))).
		Where(query))).Model(&models.File{}).
		Select(selectedFields, "(select total from ranked_scores limit 1) as total").
		Where(fmt.Sprintf("%s %s (SELECT %s FROM ranked_scores WHERE rank = ?)", orderField, op, orderField),
			max((filesQuery.Page.Value-1)*filesQuery.Limit.Value, 1)).
		Where(query).Order(getOrder(filesQuery)).Limit(filesQuery.Limit.Value)
}

func (afb *fileQueryBuilder) buildSubqueryCTE(query *gorm.DB, filesQuery *api.FilesListParams, userId int64) *gorm.DB {
	if filesQuery.DeepSearch.Value && filesQuery.Query.Value != "" && filesQuery.Path.Value != "" {
		return afb.db.Clauses(exclause.With{Recursive: true, CTEs: []exclause.CTE{{Name: "subdirs",
			Subquery: exclause.Subquery{DB: afb.db.Model(&models.File{}).Select("id", "parent_id").
				Where("id in (SELECT id FROM teldrive.get_file_from_path(?, ?, ?))", filesQuery.Path.Value, userId, true).
				Clauses(exclause.NewUnion("ALL ?",
					afb.db.Table("teldrive.files as f").Select("f.id", "f.parent_id").
						Joins("inner join subdirs ON f.parent_id = subdirs.id")))}}}})
	}
	return query
}

func getOrder(filesQuery *api.FilesListParams) string {
	orderField := utils.CamelToSnake(string(filesQuery.Sort.Value))
	return fmt.Sprintf("%s %s", orderField, strings.ToUpper(string(filesQuery.Order.Value)))
}

func getOrderOperation(filesQuery *api.FilesListParams) string {
	if filesQuery.Page.Value == 1 {
		if filesQuery.Order.Value == api.FileQueryOrderAsc {
			return ">="
		} else {
			return "<="
		}
	} else {
		if filesQuery.Order.Value == api.FileQueryOrderAsc {
			return ">"
		} else {
			return "<"
		}
	}
}
