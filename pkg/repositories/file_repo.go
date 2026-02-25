package repositories

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/table"
)

const folderCategory = "folder"

type JetFileRepository struct {
	db jetDB
}

func NewJetFileRepository(pool *pgxpool.Pool) *JetFileRepository {
	return &JetFileRepository{db: newJetDB(pool)}
}

func (r *JetFileRepository) Create(ctx context.Context, file *model.Files) error {
	now := time.Now().UTC()
	if file.CreatedAt.IsZero() {
		file.CreatedAt = now
	}
	if file.UpdatedAt.IsZero() {
		file.UpdatedAt = now
	}

	stmt := table.Files.INSERT(table.Files.AllColumns).MODEL(*file)
	_, err := r.db.exec(ctx, stmt)

	return err
}

func fileReadProjections(files *table.FilesTable) []postgres.Projection {
	return []postgres.Projection{
		files.AllColumns.Except(files.Parts),
		postgres.CAST(files.Parts).AS_TEXT().AS("files.parts"),
	}
}

func selectFilesForRead(files *table.FilesTable) postgres.SelectStatement {
	projections := fileReadProjections(files)
	return files.SELECT(projections[0], projections[1:]...)
}

func (r *JetFileRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Files, error) {
	stmt := selectFilesForRead(table.Files).FROM(table.Files).WHERE(table.Files.ID.EQ(postgres.UUID(id)))

	var out model.Files
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &out, nil
}

func (r *JetFileRepository) GetByIDAndUser(ctx context.Context, id uuid.UUID, userID int64) (*model.Files, error) {
	stmt := selectFilesForRead(table.Files).FROM(table.Files).WHERE(
		table.Files.ID.EQ(postgres.UUID(id)).AND(table.Files.UserID.EQ(postgres.Int64(userID))),
	)

	var out model.Files
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &out, nil
}

func (r *JetFileRepository) GetByChannelID(ctx context.Context, channelID int64) ([]model.Files, error) {
	stmt := selectFilesForRead(table.Files).FROM(table.Files).WHERE(
		table.Files.ChannelID.EQ(postgres.Int64(channelID)).
			AND(table.Files.Type.EQ(postgres.String("file"))),
	)

	var out []model.Files
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return []model.Files{}, nil
		}
		return nil, err
	}

	return out, nil
}

func (r *JetFileRepository) GetActiveByNameAndParent(ctx context.Context, userID int64, name string, parentID *uuid.UUID) (*model.Files, error) {
	whereExpr := table.Files.UserID.EQ(postgres.Int64(userID)).
		AND(table.Files.Name.EQ(postgres.String(name))).
		AND(table.Files.Status.EQ(postgres.String("active")))
	if parentID == nil {
		whereExpr = whereExpr.AND(table.Files.ParentID.IS_NULL())
	} else {
		whereExpr = whereExpr.AND(table.Files.ParentID.EQ(postgres.UUID(*parentID)))
	}

	stmt := selectFilesForRead(table.Files).FROM(table.Files).WHERE(whereExpr)

	var out model.Files
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &out, nil
}

func (r *JetFileRepository) Update(ctx context.Context, id uuid.UUID, update FileUpdate) error {
	updates := make([]postgres.ColumnAssigment, 0, 12)

	if update.Name != nil {
		updates = append(updates, table.Files.Name.SET(postgres.String(*update.Name)))
	}
	if update.Type != nil {
		updates = append(updates, table.Files.Type.SET(postgres.String(*update.Type)))
	}
	if update.MimeType != nil {
		updates = append(updates, table.Files.MimeType.SET(postgres.String(*update.MimeType)))
	}
	if update.Size != nil {
		updates = append(updates, table.Files.Size.SET(postgres.Int64(*update.Size)))
	}
	if update.Status != nil {
		updates = append(updates, table.Files.Status.SET(postgres.String(*update.Status)))
	}
	if update.ParentID != nil {
		updates = append(updates, table.Files.ParentID.SET(postgres.UUID(*update.ParentID)))
	}
	if update.ChannelID != nil {
		updates = append(updates, table.Files.ChannelID.SET(postgres.Int64(*update.ChannelID)))
	}
	if update.Parts != nil {
		updates = append(updates, table.Files.Parts.SET(postgres.StringExp(postgres.CAST(postgres.String(*update.Parts)).AS("jsonb"))))
	}
	if update.Encrypted != nil {
		updates = append(updates, table.Files.Encrypted.SET(postgres.Bool(*update.Encrypted)))
	}
	if update.Category != nil {
		updates = append(updates, table.Files.Category.SET(postgres.String(*update.Category)))
	}
	if update.Hash != nil {
		updates = append(updates, table.Files.Hash.SET(postgres.String(*update.Hash)))
	}

	updatedAt := time.Now().UTC()
	if update.UpdatedAt != nil {
		updatedAt = update.UpdatedAt.UTC()
	}
	updates = append(updates, table.Files.UpdatedAt.SET(postgres.TimestampT(updatedAt)))

	stmt := table.Files.UPDATE().WHERE(table.Files.ID.EQ(postgres.UUID(id)))
	stmt = stmt.SET(updates[0], assignmentArgs(updates[1:])...)

	_, err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetFileRepository) Delete(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}

	idExprs := make([]postgres.Expression, 0, len(ids))
	for _, id := range ids {
		idExprs = append(idExprs, postgres.UUID(id))
	}

	stmt := table.Files.DELETE().WHERE(table.Files.ID.IN(idExprs...))
	_, err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetFileRepository) MoveSingle(ctx context.Context, id uuid.UUID, userID int64, parentID *uuid.UUID, name *string) error {
	updateModel := model.Files{ParentID: parentID}
	var stmt postgres.UpdateStatement
	if name != nil {
		updateModel.Name = *name
		stmt = table.Files.UPDATE(table.Files.ParentID, table.Files.Name).MODEL(updateModel)
	} else {
		stmt = table.Files.UPDATE(table.Files.ParentID).MODEL(updateModel)
	}

	stmt = stmt.WHERE(
		table.Files.ID.EQ(postgres.UUID(id)).
			AND(table.Files.UserID.EQ(postgres.Int64(userID))),
	)

	_, err := r.db.exec(ctx, stmt)
	return err
}

func (r *JetFileRepository) MoveBulk(ctx context.Context, ids []uuid.UUID, userID int64, parentID *uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}

	idExprs := make([]postgres.Expression, 0, len(ids))
	for _, id := range ids {
		idExprs = append(idExprs, postgres.UUID(id))
	}

	updateModel := model.Files{ParentID: parentID}
	stmt := table.Files.UPDATE(table.Files.ParentID).
		MODEL(updateModel).
		WHERE(
			table.Files.ID.IN(idExprs...).
				AND(table.Files.UserID.EQ(postgres.Int64(userID))),
		)

	_, err := r.db.exec(ctx, stmt)
	return err
}

func (r *JetFileRepository) GetFullPath(ctx context.Context, fileID uuid.UUID) (string, error) {
	segments := []string{}
	currentID := fileID

	for depth := 0; depth < 512; depth++ {
		item, err := r.GetByID(ctx, currentID)
		if err != nil {
			return "", err
		}

		segments = append(segments, item.Name)
		if item.ParentID == nil {
			break
		}
		currentID = *item.ParentID
	}

	if len(segments) == 0 {
		return "", ErrNotFound
	}

	for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
		segments[i], segments[j] = segments[j], segments[i]
	}

	if len(segments) > 0 && segments[0] == "root" {
		segments = segments[1:]
	}

	if len(segments) == 0 {
		return "/", nil
	}

	return "/" + strings.Join(segments, "/"), nil
}

func (r *JetFileRepository) CategoryStats(ctx context.Context, userID int64) ([]CategoryStats, error) {
	stmt := table.Files.SELECT(
		table.Files.Category.AS("category_stats.category"),
		postgres.COUNT(table.Files.ID).AS("category_stats.total_files"),
		postgres.CAST(postgres.COALESCE(postgres.SUM(table.Files.Size), postgres.Int64(0))).AS_BIGINT().AS("category_stats.total_size"),
	).FROM(table.Files).WHERE(
		table.Files.UserID.EQ(postgres.Int64(userID)).
			AND(table.Files.Type.EQ(postgres.String("file"))).
			AND(table.Files.Status.EQ(postgres.String("active"))),
	).GROUP_BY(table.Files.Category).ORDER_BY(table.Files.Category.ASC())

	var stats []CategoryStats
	if err := r.db.query(ctx, stmt, &stats); err != nil {
		return nil, err
	}

	return stats, nil
}

func (r *JetFileRepository) DeleteBulk(ctx context.Context, fileIDs []uuid.UUID, userID int64, targetStatus string) error {
	if len(fileIDs) == 0 {
		return nil
	}

	idExprs := make([]postgres.Expression, 0, len(fileIDs))
	for _, id := range fileIDs {
		idExprs = append(idExprs, postgres.UUID(id))
	}

	subtreeID := postgres.StringColumn("id")
	subtree := postgres.CTE("subtree", subtreeID)

	stmt := postgres.WITH_RECURSIVE(
		subtree.AS(
			postgres.SELECT(table.Files.ID).
				FROM(table.Files).
				WHERE(
					table.Files.ID.IN(idExprs...).
						AND(table.Files.UserID.EQ(postgres.Int64(userID))),
				).
				UNION_ALL(
					postgres.SELECT(table.Files.ID).
						FROM(
							table.Files.INNER_JOIN(subtree, table.Files.ParentID.EQ(subtreeID.From(subtree))),
						),
				),
		),
	)(
		table.Files.UPDATE().
			SET(
				table.Files.Status.SET(postgres.String(targetStatus)),
				table.Files.UpdatedAt.SET(postgres.TimestampT(time.Now().UTC())),
			).
			WHERE(table.Files.ID.IN(subtree.SELECT(subtreeID.From(subtree)))),
	)

	_, err := r.db.exec(ctx, stmt)
	return err
}

func (r *JetFileRepository) CreateDirectories(ctx context.Context, userID int64, path string) (*uuid.UUID, error) {
	if !strings.HasPrefix(path, "/root") {
		path = "/root/" + strings.Trim(path, "/")
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return nil, ErrNotFound
	}

	var parentID *uuid.UUID
	for _, name := range parts {
		item, err := r.GetActiveByNameAndParent(ctx, userID, name, parentID)
		if err == nil {
			current := item.ID
			parentID = &current
			continue
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}

		now := time.Now().UTC()
		status := "active"
		typeFolder := "folder"
		mimeType := "drive/folder"
		newID := uuid.New()
		newFolder := &model.Files{
			ID:        newID,
			Name:      name,
			Type:      typeFolder,
			MimeType:  mimeType,
			UserID:    userID,
			Status:    &status,
			ParentID:  parentID,
			Encrypted: false,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := r.Create(ctx, newFolder); err != nil {
			return nil, err
		}
		parentID = &newID
	}

	return parentID, nil
}

func (r *JetFileRepository) Transaction(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx, repo FileRepository) error) error {
	if r.db.tx != nil {
		return fmt.Errorf("nested transactions are not supported")
	}

	tx, err := r.db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	txRepo := &JetFileRepository{db: jetDB{pool: r.db.pool, tx: tx}}
	if err := fn(ctx, tx, txRepo); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *JetFileRepository) ResolvePathID(ctx context.Context, path string, userID int64) (*uuid.UUID, error) {
	if !strings.HasPrefix(path, "/root") {
		path = "/root/" + strings.Trim(path, "/")
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return nil, ErrNotFound
	}

	var parentID *uuid.UUID
	for _, name := range parts {
		item, err := r.GetActiveByNameAndParent(ctx, userID, name, parentID)
		if err != nil {
			return nil, err
		}
		current := item.ID
		parentID = &current
	}

	return parentID, nil
}

func (r *JetFileRepository) List(ctx context.Context, params FileQueryParams) ([]model.Files, error) {
	if params.UserID <= 0 {
		return nil, nil
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}

	files := table.Files.AS("files")
	orderDir := fileQueryOrder(params.Order)

	conditions := []postgres.BoolExpression{files.UserID.EQ(postgres.Int64(params.UserID))}
	if params.Status != "" {
		conditions = append(conditions, files.Status.EQ(postgres.String(params.Status)))
	}

	operation := strings.ToLower(params.Operation)
	var recursiveCTEs []postgres.CommonTableExpression

	switch operation {
	case "list":
		if params.Path != "" && params.ParentID == "" {
			id, err := r.ResolvePathID(ctx, params.Path, params.UserID)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					return []model.Files{}, nil
				}

				return nil, err
			}
			conditions = append(conditions, files.ParentID.EQ(postgres.UUID(*id)))
		}
		if params.ParentID != "" {
			parentID, err := uuid.Parse(params.ParentID)
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, files.ParentID.EQ(postgres.UUID(parentID)))
		}
	case "find":
		if params.DeepSearch && params.Query != "" && params.Path != "" {
			pathID, err := r.ResolvePathID(ctx, params.Path, params.UserID)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, err
			}

			subdirs := postgres.CTE("subdirs", files.ID)
			subdirsID := files.ID.From(subdirs)

			anchor := postgres.SELECT(files.ID).FROM(files)
			if pathID == nil {
				anchor = anchor.WHERE(files.ParentID.IS_NULL())
			} else {
				anchor = anchor.WHERE(files.ID.EQ(postgres.UUID(*pathID)))
			}

			recursive := postgres.SELECT(files.ID).FROM(
				files.INNER_JOIN(subdirs, files.ParentID.EQ(subdirsID)),
			)

			recursiveCTEs = append(recursiveCTEs, subdirs.AS(anchor.UNION_ALL(recursive)))
			conditions = append(conditions, files.ID.IN(subdirs.SELECT(subdirsID)))
		}

		if params.UpdatedAt != "" {
			dateFilters, err := parseDateFilters(params.UpdatedAt)
			if err != nil {
				return nil, err
			}
			for _, filter := range dateFilters {
				switch filter.op {
				case ">=":
					conditions = append(conditions, files.UpdatedAt.GT_EQ(postgres.TimestampT(filter.value)))
				case "<=":
					conditions = append(conditions, files.UpdatedAt.LT_EQ(postgres.TimestampT(filter.value)))
				case ">":
					conditions = append(conditions, files.UpdatedAt.GT(postgres.TimestampT(filter.value)))
				case "<":
					conditions = append(conditions, files.UpdatedAt.LT(postgres.TimestampT(filter.value)))
				default:
					conditions = append(conditions, files.UpdatedAt.EQ(postgres.TimestampT(filter.value)))
				}
			}
		}

		if params.Query != "" {
			switch strings.ToLower(params.SearchType) {
			case "regex":
				conditions = append(conditions, postgres.RawBool(
					"files.name &~ #searchQuery",
					postgres.RawArgs{"#searchQuery": params.Query},
				))
			default:
				conditions = append(conditions, postgres.RawBool(
					"teldrive.clean_name(files.name) &@~ teldrive.clean_name(#searchQuery)",
					postgres.RawArgs{"#searchQuery": params.Query},
				))
			}
		}

		if len(params.Category) > 0 {
			parts := make([]postgres.BoolExpression, 0, len(params.Category))
			for _, category := range params.Category {
				if category == folderCategory {
					parts = append(parts, files.Type.EQ(postgres.String(category)))
				} else {
					parts = append(parts, files.Category.EQ(postgres.String(category)))
				}
			}
			conditions = append(conditions, postgres.OR(parts...))
		}

		if params.Name != "" {
			conditions = append(conditions, files.Name.EQ(postgres.String(params.Name)))
		}

		if params.ParentID != "" {
			if params.ParentID == "nil" {
				conditions = append(conditions, files.ParentID.IS_NULL())
			} else {
				parentID, err := uuid.Parse(params.ParentID)
				if err != nil {
					return nil, err
				}
				conditions = append(conditions, files.ParentID.EQ(postgres.UUID(parentID)))
			}
		}

		if params.ParentID == "" && params.Path != "" && params.Query == "" {
			id, err := r.ResolvePathID(ctx, params.Path, params.UserID)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					return []model.Files{}, nil
				}

				return nil, err
			}

			conditions = append(conditions, files.ParentID.EQ(postgres.UUID(*id)))
		}

		if params.Type != "" {
			conditions = append(conditions, files.Type.EQ(postgres.String(params.Type)))
		}

		if params.Shared {
			conditions = append(conditions, files.ID.IN(
				postgres.SELECT(table.FileShares.FileID).FROM(table.FileShares).WHERE(
					table.FileShares.UserID.EQ(postgres.Int64(params.UserID)),
				),
			))
		}
	}

	cursorCondition, err := buildFileCursorCondition(files, params.Sort, orderDir, params.Cursor)
	if err != nil {
		return nil, err
	}
	if cursorCondition != nil {
		conditions = append(conditions, cursorCondition)
	}

	whereExpr := postgres.AND(conditions...)

	listStmt := files.SELECT(files.AllColumns.Except(files.Parts)).FROM(files).WHERE(whereExpr).LIMIT(int64(limit))

	listStmt = applyFileSort(listStmt, files, params.Sort, orderDir)

	var listQuery postgres.Statement = listStmt
	if len(recursiveCTEs) > 0 {
		listQuery = postgres.WITH_RECURSIVE(recursiveCTEs...)(listStmt)
	}

	var results []model.Files
	if err := r.db.query(ctx, listQuery, &results); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return []model.Files{}, nil
		}

		return nil, err
	}

	return results, nil
}

type dateFilter struct {
	op    string
	value time.Time
}

func parseDateFilters(input string) ([]dateFilter, error) {
	parts := strings.Split(input, ",")
	filters := make([]dateFilter, 0, len(parts))

	for _, part := range parts {
		chunks := strings.Split(part, ":")
		if len(chunks) != 2 {
			continue
		}

		dateValue, err := time.Parse(time.DateOnly, chunks[1])
		if err != nil {
			return nil, err
		}

		op := "="
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

		filters = append(filters, dateFilter{op: op, value: dateValue.UTC()})
	}

	return filters, nil
}

func fileQuerySortField(sort string) string {
	switch strings.ToLower(sort) {
	case "name":
		return "files.name"
	case "updatedat":
		return "files.updated_at"
	case "updated_at":
		return "files.updated_at"
	case "size":
		return "files.size"
	case "id":
		return "files.id"
	default:
		return "files.updated_at"
	}
}

func fileQueryOrder(order string) string {
	if strings.EqualFold(order, "asc") {
		return "ASC"
	}

	return "DESC"
}

func applyFileSort(
	stmt postgres.SelectStatement,
	files *table.FilesTable,
	sort string,
	orderDir string,
) postgres.SelectStatement {
	asc := orderDir == "ASC"

	switch strings.ToLower(sort) {
	case "name":
		if asc {
			return stmt.ORDER_BY(files.Name.ASC(), files.ID.ASC())
		}
		return stmt.ORDER_BY(files.Name.DESC(), files.ID.DESC())
	case "size":
		if asc {
			return stmt.ORDER_BY(files.Size.ASC(), files.ID.ASC())
		}
		return stmt.ORDER_BY(files.Size.DESC(), files.ID.DESC())
	case "id":
		if asc {
			return stmt.ORDER_BY(files.ID.ASC())
		}
		return stmt.ORDER_BY(files.ID.DESC())
	default:
		if asc {
			return stmt.ORDER_BY(files.UpdatedAt.ASC(), files.ID.ASC())
		}
		return stmt.ORDER_BY(files.UpdatedAt.DESC(), files.ID.DESC())
	}
}

func buildFileCursorCondition(
	files *table.FilesTable,
	sort string,
	orderDir string,
	cursor string,
) (postgres.BoolExpression, error) {
	if cursor == "" {
		return nil, nil
	}

	splitAt := strings.LastIndex(cursor, ":")
	if splitAt <= 0 || splitAt >= len(cursor)-1 {
		return nil, nil
	}

	cursorValue := cursor[:splitAt]
	cursorIDText := cursor[splitAt+1:]

	cursorID, err := uuid.Parse(cursorIDText)
	if err != nil {
		return nil, nil
	}

	asc := orderDir == "ASC"
	idExpr := postgres.UUID(cursorID)

	switch strings.ToLower(sort) {
	case "name":
		valueExpr := postgres.String(cursorValue)
		if asc {
			return files.Name.GT(valueExpr).OR(files.Name.EQ(valueExpr).AND(files.ID.GT(idExpr))), nil
		}
		return files.Name.LT(valueExpr).OR(files.Name.EQ(valueExpr).AND(files.ID.LT(idExpr))), nil
	case "size":
		size, err := strconv.ParseInt(cursorValue, 10, 64)
		if err != nil {
			return nil, nil
		}
		valueExpr := postgres.Int64(size)
		if asc {
			return files.Size.GT(valueExpr).OR(files.Size.EQ(valueExpr).AND(files.ID.GT(idExpr))), nil
		}
		return files.Size.LT(valueExpr).OR(files.Size.EQ(valueExpr).AND(files.ID.LT(idExpr))), nil
	case "id":
		idValue, err := uuid.Parse(cursorValue)
		if err != nil {
			idValue = cursorID
		}
		valueExpr := postgres.UUID(idValue)
		if asc {
			return files.ID.GT(valueExpr), nil
		}
		return files.ID.LT(valueExpr), nil
	default:
		updatedAt, err := time.Parse(time.RFC3339Nano, cursorValue)
		if err != nil {
			return nil, nil
		}
		valueExpr := postgres.TimestampT(updatedAt)
		if asc {
			return files.UpdatedAt.GT(valueExpr).OR(files.UpdatedAt.EQ(valueExpr).AND(files.ID.GT(idExpr))), nil
		}
		return files.UpdatedAt.LT(valueExpr).OR(files.UpdatedAt.EQ(valueExpr).AND(files.ID.LT(idExpr))), nil
	}
}
