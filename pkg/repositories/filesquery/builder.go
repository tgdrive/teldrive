package filesquery

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/table"
)

type Query struct {
	UserID      int64
	Operation   Operation
	Status      string
	ParentID    *uuid.UUID
	ParentIsNil bool
	Path        string
	Name        string
	Type        string
	Categories  []string
	Search      SearchParams
	UpdatedAt   []DateFilter
	Shared      bool
	Sort        SortField
	Order       SortOrder
	Cursor      string
	Limit       int
}

type Operation string

const (
	OpList Operation = "list"
	OpFind Operation = "find"
)

type SearchParams struct {
	Query      string
	SearchType SearchType
	DeepSearch bool
}

type SearchType string

const (
	SearchTypeDefault SearchType = ""
	SearchTypeText    SearchType = "text"
	SearchTypeRegex   SearchType = "regex"
)

type DateFilter struct {
	Op    string // "=", "!=", ">", "<", ">=", "<="
	Value time.Time
}

type SortField string

const (
	SortFieldName      SortField = "name"
	SortFieldSize      SortField = "size"
	SortFieldID        SortField = "id"
	SortFieldUpdatedAt SortField = "updated_at"
)

type SortOrder string

const (
	SortOrderAsc  SortOrder = "ASC"
	SortOrderDesc SortOrder = "DESC"
)

type Builder struct {
	filesTable *table.FilesTable
}

func NewBuilder() *Builder {
	return &Builder{
		filesTable: table.Files.AS("files"),
	}
}

func (b *Builder) Build(q Query) (postgres.Statement, CursorEncoder, error) {
	q = b.normalize(q)
	if err := b.validate(q); err != nil {
		return nil, CursorEncoder{}, err
	}

	cursorEncoder := newCursorEncoder(q.Sort, q.Order, q.Cursor)
	conditions := b.buildBaseConditions(q)
	if cursorCondition, err := cursorEncoder.Decode(q.Cursor); err != nil {
		return nil, CursorEncoder{}, err
	} else if cursorCondition != nil {
		condition, err := cursorCondition.BuildCondition(b.filesTable)
		if err != nil {
			return nil, CursorEncoder{}, err
		}
		conditions = append(conditions, condition)
	}
	recursiveCTEs := b.buildRecursiveCTEs(q, conditions)
	whereExpr := postgres.AND(conditions...)

	listStmt := b.filesTable.SELECT(
		b.filesTable.AllColumns.Except(b.filesTable.Parts),
	).FROM(b.filesTable).WHERE(whereExpr).LIMIT(int64(q.Limit))

	listStmt = b.applySort(listStmt, q.Sort, q.Order)

	var stmt postgres.Statement = listStmt
	if len(recursiveCTEs) > 0 {
		stmt = postgres.WITH_RECURSIVE(recursiveCTEs...)(listStmt)
	}

	return stmt, cursorEncoder, nil
}

func (b *Builder) normalize(q Query) Query {
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 100
	}
	return q
}

func (b *Builder) validate(q Query) error {
	if q.UserID <= 0 {
		return fmt.Errorf("userID is required")
	}
	return nil
}

func (b *Builder) buildBaseConditions(q Query) []postgres.BoolExpression {
	conditions := []postgres.BoolExpression{
		b.filesTable.UserID.EQ(postgres.Int64(q.UserID)),
	}

	if q.Status != "" {
		conditions = append(conditions, b.filesTable.Status.EQ(postgres.String(q.Status)))
	}

	switch q.Operation {
	case OpList:
		b.buildListConditions(q, &conditions)
	case OpFind:
		b.buildFindConditions(q, &conditions)
	}

	return conditions
}

func (b *Builder) buildListConditions(q Query, conditions *[]postgres.BoolExpression) {
	if q.ParentID != nil {
		*conditions = append(*conditions, b.filesTable.ParentID.EQ(postgres.UUID(*q.ParentID)))
	}
}

func (b *Builder) buildFindConditions(q Query, conditions *[]postgres.BoolExpression) {

	for _, filter := range q.UpdatedAt {
		*conditions = append(*conditions, b.buildDateCondition(filter))
	}

	if q.Search.Query != "" {
		*conditions = append(*conditions, b.buildSearchCondition(q.Search))
	}

	if len(q.Categories) > 0 {
		*conditions = append(*conditions, b.buildCategoryCondition(q.Categories))
	}

	if q.Name != "" {
		*conditions = append(*conditions, b.filesTable.Name.EQ(postgres.String(q.Name)))
	}

	if q.ParentIsNil {
		*conditions = append(*conditions, b.filesTable.ParentID.IS_NULL())
	} else if q.ParentID != nil {
		*conditions = append(*conditions, b.filesTable.ParentID.EQ(postgres.UUID(*q.ParentID)))
	}

	if q.Type != "" {
		*conditions = append(*conditions, b.filesTable.Type.EQ(postgres.String(q.Type)))
	}

	if q.Shared {
		*conditions = append(*conditions, b.filesTable.ID.IN(
			postgres.SELECT(table.FileShares.FileID).FROM(table.FileShares).WHERE(
				table.FileShares.UserID.EQ(postgres.Int64(q.UserID)),
			),
		))
	}
}

func (b *Builder) buildRecursiveCTEs(q Query, conditions []postgres.BoolExpression) []postgres.CommonTableExpression {
	if !q.Search.DeepSearch || q.Search.Query == "" || q.Path == "" {
		return nil
	}

	if q.ParentID == nil {
		return nil
	}

	subdirs := postgres.CTE("subdirs", b.filesTable.ID)
	subdirsID := b.filesTable.ID.From(subdirs)

	anchor := postgres.SELECT(b.filesTable.ID).FROM(b.filesTable)
	anchor = anchor.WHERE(b.filesTable.ID.EQ(postgres.UUID(*q.ParentID)))

	recursive := postgres.SELECT(b.filesTable.ID).FROM(
		b.filesTable.INNER_JOIN(subdirs, b.filesTable.ParentID.EQ(subdirsID)),
	)

	return []postgres.CommonTableExpression{
		subdirs.AS(anchor.UNION_ALL(recursive)),
	}
}

func (b *Builder) buildDateCondition(filter DateFilter) postgres.BoolExpression {
	value := postgres.TimestampT(filter.Value)
	switch filter.Op {
	case ">=":
		return b.filesTable.UpdatedAt.GT_EQ(value)
	case "<=":
		return b.filesTable.UpdatedAt.LT_EQ(value)
	case ">":
		return b.filesTable.UpdatedAt.GT(value)
	case "<":
		return b.filesTable.UpdatedAt.LT(value)
	default:
		return b.filesTable.UpdatedAt.EQ(value)
	}
}

func (b *Builder) buildSearchCondition(search SearchParams) postgres.BoolExpression {
	switch search.SearchType {
	case SearchTypeRegex:
		return postgres.RawBool(
			"files.name &~ #searchQuery",
			postgres.RawArgs{"#searchQuery": search.Query},
		)
	default:
		return postgres.RawBool(
			"teldrive.clean_name(files.name) &@~ teldrive.clean_name(#searchQuery)",
			postgres.RawArgs{"#searchQuery": search.Query},
		)
	}
}

func (b *Builder) buildCategoryCondition(categories []string) postgres.BoolExpression {
	var parts []postgres.BoolExpression
	for _, category := range categories {
		if category == "folder" {
			parts = append(parts, b.filesTable.Type.EQ(postgres.String(category)))
		} else {
			parts = append(parts, b.filesTable.Category.EQ(postgres.String(category)))
		}
	}
	return postgres.OR(parts...)
}

func (b *Builder) applySort(stmt postgres.SelectStatement, sort SortField, order SortOrder) postgres.SelectStatement {
	asc := order == SortOrderAsc

	switch sort {
	case SortFieldName:
		if asc {
			return stmt.ORDER_BY(b.filesTable.Name.ASC(), b.filesTable.ID.ASC())
		}
		return stmt.ORDER_BY(b.filesTable.Name.DESC(), b.filesTable.ID.DESC())
	case SortFieldSize:
		if asc {
			return stmt.ORDER_BY(b.filesTable.Size.ASC(), b.filesTable.ID.ASC())
		}
		return stmt.ORDER_BY(b.filesTable.Size.DESC(), b.filesTable.ID.DESC())
	case SortFieldID:
		if asc {
			return stmt.ORDER_BY(b.filesTable.ID.ASC())
		}
		return stmt.ORDER_BY(b.filesTable.ID.DESC())
	default:
		if asc {
			return stmt.ORDER_BY(b.filesTable.UpdatedAt.ASC(), b.filesTable.ID.ASC())
		}
		return stmt.ORDER_BY(b.filesTable.UpdatedAt.DESC(), b.filesTable.ID.DESC())
	}
}

type CursorEncoder struct {
	sortField SortField
	order     SortOrder
}

func newCursorEncoder(sortField SortField, order SortOrder, currentCursor string) CursorEncoder {
	return CursorEncoder{
		sortField: sortField,
		order:     order,
	}
}

func (c CursorEncoder) Encode(file model.Files) string {
	var cursorVal string
	switch c.sortField {
	case SortFieldName:
		cursorVal = file.Name
	case SortFieldSize:
		if file.Size != nil {
			cursorVal = strconv.FormatInt(*file.Size, 10)
		}
	case SortFieldID:
		cursorVal = file.ID.String()
	default:
		cursorVal = file.UpdatedAt.Format(time.RFC3339Nano)
	}
	return fmt.Sprintf("%s:%s", cursorVal, file.ID.String())
}

func (c CursorEncoder) Decode(cursor string) (*CursorCondition, error) {
	if cursor == "" {
		return nil, nil
	}

	splitAt := strings.LastIndex(cursor, ":")
	if splitAt <= 0 || splitAt >= len(cursor)-1 {
		return nil, fmt.Errorf("invalid cursor format")
	}

	cursorValue := cursor[:splitAt]
	cursorIDText := cursor[splitAt+1:]

	cursorID, err := uuid.Parse(cursorIDText)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor ID: %w", err)
	}

	asc := c.order == SortOrderAsc

	return &CursorCondition{
		CursorValue: cursorValue,
		CursorID:    cursorID,
		SortField:   c.sortField,
		Ascending:   asc,
	}, nil
}

type CursorCondition struct {
	CursorValue string
	CursorID    uuid.UUID
	SortField   SortField
	Ascending   bool
}

func (cc CursorCondition) BuildCondition(files *table.FilesTable) (postgres.BoolExpression, error) {
	idExpr := postgres.UUID(cc.CursorID)

	switch cc.SortField {
	case SortFieldName:
		valueExpr := postgres.String(cc.CursorValue)
		if cc.Ascending {
			return files.Name.GT(valueExpr).OR(
				files.Name.EQ(valueExpr).AND(files.ID.GT(idExpr)),
			), nil
		}
		return files.Name.LT(valueExpr).OR(
			files.Name.EQ(valueExpr).AND(files.ID.LT(idExpr)),
		), nil
	case SortFieldSize:
		size, err := strconv.ParseInt(cc.CursorValue, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor size: %w", err)
		}
		valueExpr := postgres.Int64(size)
		if cc.Ascending {
			return files.Size.GT(valueExpr).OR(
				files.Size.EQ(valueExpr).AND(files.ID.GT(idExpr)),
			), nil
		}
		return files.Size.LT(valueExpr).OR(
			files.Size.EQ(valueExpr).AND(files.ID.LT(idExpr)),
		), nil
	case SortFieldID:
		valueExpr := postgres.UUID(cc.CursorID)
		if cc.Ascending {
			return files.ID.GT(valueExpr), nil
		}
		return files.ID.LT(valueExpr), nil
	default:
		updatedAt, err := time.Parse(time.RFC3339Nano, cc.CursorValue)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor time: %w", err)
		}
		valueExpr := postgres.TimestampT(updatedAt)
		if cc.Ascending {
			return files.UpdatedAt.GT(valueExpr).OR(
				files.UpdatedAt.EQ(valueExpr).AND(files.ID.GT(idExpr)),
			), nil
		}
		return files.UpdatedAt.LT(valueExpr).OR(
			files.UpdatedAt.EQ(valueExpr).AND(files.ID.LT(idExpr)),
		), nil
	}
}

func ParseDateFilters(input string) ([]DateFilter, error) {
	if input == "" {
		return nil, nil
	}

	parts := strings.Split(input, ",")
	filters := make([]DateFilter, 0, len(parts))

	for _, part := range parts {
		chunks := strings.Split(part, ":")
		if len(chunks) != 2 {
			continue
		}

		dateValue, err := time.Parse(time.DateOnly, chunks[1])
		if err != nil {
			return nil, fmt.Errorf("invalid date format: %w", err)
		}

		var op string
		switch chunks[0] {
		case "gte":
			op = ">="
		case "lte":
			op = "<="
		case "eq":
			op = "="
		case "gt":
			op = ">"
		case "lt":
			op = "<"
		default:
			continue
		}

		filters = append(filters, DateFilter{Op: op, Value: dateValue.UTC()})
	}

	return filters, nil
}
