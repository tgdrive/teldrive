package filesquery

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/internal/database/jet/gen/model"
)

func TestBuilder_Build_ListByParentID(t *testing.T) {
	builder := NewBuilder()
	parentID := uuid.New()

	query := Query{
		UserID:    1,
		Operation: OpList,
		ParentID:  &parentID,
		Limit:     20,
	}

	stmt, encoder, err := builder.Build(query)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	sql, args := stmt.Sql()
	if !strings.Contains(sql, "files.user_id") {
		t.Errorf("Build() should contain user_id filter, got: %s", sql)
	}
	if !strings.Contains(sql, "files.parent_id") {
		t.Errorf("Build() should contain parent_id filter, got: %s", sql)
	}
	if len(args) == 0 {
		t.Errorf("Build() should have args")
	}

	// Test cursor encoder
	cursor := encoder.Encode(model.Files{
		ID:        uuid.New(),
		Name:      "test.txt",
		UpdatedAt: time.Now(),
	})
	if cursor == "" {
		t.Error("Encoder.Encode() should return non-empty string")
	}
}

func TestBuilder_Build_FindQuery(t *testing.T) {
	builder := NewBuilder()

	query := Query{
		UserID:    1,
		Operation: OpFind,
		Search: SearchParams{
			Query:      "document",
			SearchType: SearchTypeText,
		},
		Limit: 20,
	}

	stmt, _, err := builder.Build(query)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	sql, args := stmt.Sql()
	if !strings.Contains(sql, "teldrive.clean_name") {
		t.Errorf("Build() should contain full-text search, got: %s", sql)
	}

	found := false
	for _, arg := range args {
		if arg == "document" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Build() should contain search query in args, got args: %v", args)
	}
}

func TestBuilder_Build_FindRegex(t *testing.T) {
	builder := NewBuilder()

	query := Query{
		UserID:    1,
		Operation: OpFind,
		Search: SearchParams{
			Query:      "^test.*\\.log$",
			SearchType: SearchTypeRegex,
		},
		Limit: 20,
	}

	stmt, _, err := builder.Build(query)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	sql, _ := stmt.Sql()
	if !strings.Contains(sql, "&~") {
		t.Errorf("Build() should contain regex operator, got: %s", sql)
	}
}

func TestBuilder_Build_CategoryFolder(t *testing.T) {
	builder := NewBuilder()

	query := Query{
		UserID:     1,
		Operation:  OpFind,
		Categories: []string{"folder"},
		Limit:      20,
	}

	stmt, _, err := builder.Build(query)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	sql, _ := stmt.Sql()
	if !strings.Contains(sql, "files.type") {
		t.Errorf("Build() should filter by type for folder category, got: %s", sql)
	}
}

func TestBuilder_Build_CategoryFileTypes(t *testing.T) {
	builder := NewBuilder()

	query := Query{
		UserID:     1,
		Operation:  OpFind,
		Categories: []string{"video", "audio", "image"},
		Limit:      20,
	}

	stmt, _, err := builder.Build(query)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	sql, _ := stmt.Sql()
	if !strings.Contains(sql, "files.category") {
		t.Errorf("Build() should filter by category for file types, got: %s", sql)
	}
	if !strings.Contains(sql, "OR") {
		t.Errorf("Build() should use OR for multiple categories, got: %s", sql)
	}
}

func TestBuilder_Build_UpdatedAtFilters(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		op     string
	}{
		{"gte", "gte:2024-01-01", ">="},
		{"lte", "lte:2024-01-01", "<="},
		{"gt", "gt:2024-01-01", ">"},
		{"lt", "lt:2024-01-01", "<"},
		{"eq", "eq:2024-01-01", "="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters, err := ParseDateFilters(tt.filter)
			if err != nil {
				t.Fatalf("ParseDateFilters() error = %v", err)
			}

			builder := NewBuilder()
			query := Query{
				UserID:    1,
				Operation: OpFind,
				UpdatedAt: filters,
				Limit:     20,
			}

			stmt, _, err := builder.Build(query)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}

			sql, _ := stmt.Sql()
			if !strings.Contains(sql, "files.updated_at") {
				t.Errorf("Build() should contain updated_at filter, got: %s", sql)
			}
		})
	}
}

func TestBuilder_Build_SortByName(t *testing.T) {
	tests := []struct {
		name  string
		sort  SortField
		order SortOrder
	}{
		{"name_asc", SortFieldName, SortOrderAsc},
		{"name_desc", SortFieldName, SortOrderDesc},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder()
			query := Query{
				UserID:    1,
				Operation: OpFind,
				Sort:      tt.sort,
				Order:     tt.order,
				Limit:     20,
			}

			stmt, _, err := builder.Build(query)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}

			sql, _ := stmt.Sql()
			if !strings.Contains(sql, "ORDER BY") {
				t.Errorf("Build() should contain ORDER BY, got: %s", sql)
			}
		})
	}
}

func TestBuilder_Build_SortByUpdatedAt(t *testing.T) {
	builder := NewBuilder()

	query := Query{
		UserID:    1,
		Operation: OpFind,
		Sort:      SortFieldUpdatedAt,
		Order:     SortOrderDesc,
		Cursor:    "2024-01-01T00:00:00Z:" + uuid.New().String(),
		Limit:     20,
	}

	stmt, encoder, err := builder.Build(query)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	sql, _ := stmt.Sql()
	if !strings.Contains(sql, "ORDER BY") {
		t.Errorf("Build() should contain ORDER BY, got: %s", sql)
	}

	// Test cursor decoding
	cond, err := encoder.Decode(query.Cursor)
	if err != nil {
		t.Errorf("Encoder.Decode() error = %v", err)
	}
	if cond == nil {
		t.Error("Encoder.Decode() should return non-nil condition")
	}
}

func TestBuilder_Build_InvalidCursor(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name   string
		cursor string
	}{
		{"empty", ""},
		{"no_colon", "invalid"},
		{"empty_value", ":" + uuid.New().String()},
		{"invalid_uuid", "value:not-a-uuid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := Query{
				UserID:    1,
				Operation: OpFind,
				Cursor:    tt.cursor,
				Limit:     20,
			}

			stmt, encoder, err := builder.Build(query)
			if err != nil && tt.cursor != "" && tt.name == "empty" {
				t.Errorf("Build() unexpected error = %v", err)
			}
			if err == nil && tt.cursor != "" && tt.name != "empty" {
				t.Error("Build() should return error for invalid cursor")
			}

			if tt.cursor != "" {
				_, err := encoder.Decode(tt.cursor)
				if err == nil && tt.cursor != "" {
					t.Error("Encoder.Decode() should return error for invalid cursor")
				}
			}

			// Empty cursor should still produce valid statement
			if tt.cursor == "" && stmt == nil {
				t.Error("Build() should return valid statement for empty cursor")
			}
		})
	}
}

func TestBuilder_Build_SharedFilter(t *testing.T) {
	builder := NewBuilder()

	query := Query{
		UserID:    1,
		Operation: OpFind,
		Shared:    true,
		Limit:     20,
	}

	stmt, _, err := builder.Build(query)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	sql, _ := stmt.Sql()
	if !strings.Contains(sql, "file_shares") {
		t.Errorf("Build() should contain file_shares join for shared filter, got: %s", sql)
	}
}

func TestBuilder_Build_Limit(t *testing.T) {
	tests := []struct {
		name  string
		input int
	}{
		{"default", 0},
		{"negative", -1},
		{"normal", 50},
		{"too_high", 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder()
			query := Query{
				UserID:    1,
				Operation: OpFind,
				Limit:     tt.input,
			}

			stmt, _, err := builder.Build(query)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}

			sql, _ := stmt.Sql()
			if !strings.Contains(sql, "LIMIT") {
				t.Errorf("Build() should contain LIMIT clause, got: %s", sql)
			}
		})
	}
}

func TestBuilder_Build_StatusFilter(t *testing.T) {
	builder := NewBuilder()

	query := Query{
		UserID:    1,
		Operation: OpFind,
		Status:    "active",
		Limit:     20,
	}

	stmt, _, err := builder.Build(query)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	sql, _ := stmt.Sql()
	if !strings.Contains(sql, "files.status") {
		t.Errorf("Build() should contain status filter, got: %s", sql)
	}
}

func TestBuilder_Build_TypeFilter(t *testing.T) {
	builder := NewBuilder()

	query := Query{
		UserID:    1,
		Operation: OpFind,
		Type:      "file",
		Limit:     20,
	}

	stmt, _, err := builder.Build(query)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	sql, _ := stmt.Sql()
	if !strings.Contains(sql, "files.type") {
		t.Errorf("Build() should contain type filter, got: %s", sql)
	}
}

func TestBuilder_Build_NameFilter(t *testing.T) {
	builder := NewBuilder()

	query := Query{
		UserID:    1,
		Operation: OpFind,
		Name:      "test.txt",
		Limit:     20,
	}

	stmt, _, err := builder.Build(query)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	sql, args := stmt.Sql()
	if !strings.Contains(sql, "files.name") {
		t.Errorf("Build() should contain name filter, got: %s", sql)
	}

	found := false
	for _, arg := range args {
		if arg == "test.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Build() should contain name in args, got args: %v", args)
	}
}

func TestCursorEncoder_Encode(t *testing.T) {
	encoder := newCursorEncoder(SortFieldName, SortOrderAsc, "")

	file := model.Files{
		ID:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Name: "test.txt",
	}

	cursor := encoder.Encode(file)
	expected := "test.txt:11111111-1111-1111-1111-111111111111"
	if cursor != expected {
		t.Errorf("Encode() = %v, want %v", cursor, expected)
	}
}

func TestCursorEncoder_Encode_Size(t *testing.T) {
	encoder := newCursorEncoder(SortFieldSize, SortOrderAsc, "")

	size := int64(12345)
	file := model.Files{
		ID:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Size: &size,
	}

	cursor := encoder.Encode(file)
	expected := "12345:11111111-1111-1111-1111-111111111111"
	if cursor != expected {
		t.Errorf("Encode() = %v, want %v", cursor, expected)
	}
}

func TestCursorEncoder_Decode_NameAsc(t *testing.T) {
	encoder := newCursorEncoder(SortFieldName, SortOrderAsc, "")

	cond, err := encoder.Decode("test.txt:11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if cond == nil {
		t.Fatal("Decode() should return non-nil condition")
	}

	if cond.CursorValue != "test.txt" {
		t.Errorf("Decode().CursorValue = %v, want 'test.txt'", cond.CursorValue)
	}
	if cond.Ascending != true {
		t.Errorf("Decode().Ascending = %v, want true", cond.Ascending)
	}
}

func TestCursorEncoder_Decode_UpdatedAtDesc(t *testing.T) {
	encoder := newCursorEncoder(SortFieldUpdatedAt, SortOrderDesc, "")

	cursor := "2024-01-01T12:00:00Z:11111111-1111-1111-1111-111111111111"
	cond, err := encoder.Decode(cursor)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if cond == nil {
		t.Fatal("Decode() should return non-nil condition")
	}
	if !strings.HasPrefix(cond.CursorValue, "2024-01-01") {
		t.Errorf("Decode().CursorValue = %v, want prefix '2024-01-01'", cond.CursorValue)
	}
}

func TestCursorEncoder_Decode_IDAsc(t *testing.T) {
	encoder := newCursorEncoder(SortFieldID, SortOrderAsc, "")
	id := uuid.New()

	// For ID sort, the cursor format is "idvalue:id"
	cond, err := encoder.Decode(id.String() + ":" + id.String())
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if cond == nil {
		t.Fatal("Decode() should return non-nil condition")
	}
	if cond.Ascending != true {
		t.Errorf("Decode().Ascending = %v, want true", cond.Ascending)
	}
}

func TestParseDateFilters(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		wantErr bool
	}{
		{"empty", "", 0, false},
		{"single_gte", "gte:2024-01-01", 1, false},
		{"multiple", "gte:2024-01-01,lte:2024-12-31", 2, false},
		{"invalid_date", "gte:not-a-date", 0, true},
		{"skip_invalid_op", "invalid:2024-01-01", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters, err := ParseDateFilters(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDateFilters() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(filters) != tt.wantLen {
				t.Errorf("ParseDateFilters() len = %v, want %v", len(filters), tt.wantLen)
			}
		})
	}
}

func TestBuilder_Build_UserIDRequired(t *testing.T) {
	builder := NewBuilder()

	query := Query{
		UserID:    0,
		Operation: OpFind,
		Limit:     20,
	}

	_, _, err := builder.Build(query)
	if err == nil {
		t.Error("Build() should return error for zero userID")
	}
}
