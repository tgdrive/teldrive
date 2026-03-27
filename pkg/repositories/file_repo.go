package repositories

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/internal/database/jet/gen/table"
	"github.com/tgdrive/teldrive/pkg/repositories/filesquery"
)

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
	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetFileRepository) UpsertActive(ctx context.Context, file *model.Files) error {
	now := time.Now().UTC()
	if file.CreatedAt.IsZero() {
		file.CreatedAt = now
	}
	if file.UpdatedAt.IsZero() {
		file.UpdatedAt = now
	}

	if err := r.updateActiveByNaturalKey(ctx, file); err == nil {
		return nil
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}

	stmt := table.Files.INSERT(table.Files.AllColumns).MODEL(*file)
	if err := r.db.exec(ctx, stmt); err != nil {
		if errors.Is(err, ErrConflict) {
			return r.updateActiveByNaturalKey(ctx, file)
		}
		return err
	}

	return nil
}

func (r *JetFileRepository) updateActiveByNaturalKey(ctx context.Context, file *model.Files) error {
	whereExpr := table.Files.Name.EQ(postgres.String(file.Name)).
		AND(table.Files.UserID.EQ(postgres.Int64(file.UserID))).
		AND(table.Files.Status.EQ(postgres.String("active")))
	if file.ParentID == nil {
		whereExpr = whereExpr.AND(table.Files.ParentID.IS_NULL())
	} else {
		whereExpr = whereExpr.AND(table.Files.ParentID.IS_NOT_DISTINCT_FROM(postgres.UUID(*file.ParentID)))
	}

	stmt := table.Files.UPDATE(
		table.Files.MimeType,
		table.Files.Size,
		table.Files.ChannelID,
		table.Files.Parts,
		table.Files.UpdatedAt,
		table.Files.Encrypted,
		table.Files.Category,
		table.Files.Hash,
	).SET(
		file.MimeType,
		file.Size,
		file.ChannelID,
		file.Parts,
		file.UpdatedAt,
		file.Encrypted,
		file.Category,
		file.Hash,
	).WHERE(whereExpr).RETURNING(table.Files.ID)

	query, args := stmt.Sql()
	row := r.db.executor(ctx).QueryRow(ctx, query, args...)
	if err := ScanRow(row, &file.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return normalizeDBError(err)
	}

	return nil
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
	stmt := selectFilesForRead(table.Files).FROM(table.Files).WHERE(
		table.Files.ID.EQ(postgres.UUID(id)).AND(table.Files.Status.EQ(postgres.String("active"))),
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

func (r *JetFileRepository) GetByIDAndUser(ctx context.Context, id uuid.UUID, userID int64) (*model.Files, error) {
	stmt := selectFilesForRead(table.Files).FROM(table.Files).WHERE(
		table.Files.ID.EQ(postgres.UUID(id)).
			AND(table.Files.UserID.EQ(postgres.Int64(userID))).
			AND(table.Files.Status.EQ(postgres.String("active"))),
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
	_, err := r.UpdateReturning(ctx, id, update)
	return err
}

func (r *JetFileRepository) UpdateReturning(ctx context.Context, id uuid.UUID, update FileUpdate) (*model.Files, error) {
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
		updates = append(updates, table.Files.Parts.SET(postgres.StringExp(postgres.Raw("#parts", postgres.RawArgs{"#parts": update.Parts}))))
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
	projections := fileReadProjections(table.Files)
	stmt = stmt.RETURNING(projections...)

	var out model.Files
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &out, nil
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
	err := r.db.exec(ctx, stmt)

	return err
}

func (r *JetFileRepository) MoveSingle(ctx context.Context, id uuid.UUID, userID int64, parentID *uuid.UUID, name *string) error {
	_, err := r.MoveSingleReturning(ctx, id, userID, parentID, name)
	return err
}

func (r *JetFileRepository) MoveSingleReturning(ctx context.Context, id uuid.UUID, userID int64, parentID *uuid.UUID, name *string) (*model.Files, error) {
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
	projections := fileReadProjections(table.Files)
	stmt = stmt.RETURNING(projections...)

	var out model.Files
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &out, nil
}

func (r *JetFileRepository) MoveBulk(ctx context.Context, ids []uuid.UUID, userID int64, parentID *uuid.UUID) error {
	_, err := r.MoveBulkReturning(ctx, ids, userID, parentID)
	return err
}

func (r *JetFileRepository) MoveBulkReturning(ctx context.Context, ids []uuid.UUID, userID int64, parentID *uuid.UUID) ([]model.Files, error) {
	if len(ids) == 0 {
		return nil, nil
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
	projections := fileReadProjections(table.Files)
	stmt = stmt.RETURNING(projections...)

	var out []model.Files
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return []model.Files{}, nil
		}
		return nil, err
	}

	return out, nil
}

func (r *JetFileRepository) GetFullPath(ctx context.Context, fileID uuid.UUID) (string, error) {
	ancestorsID := postgres.StringColumn("id")
	ancestorsParentID := postgres.StringColumn("parent_id")
	ancestorsName := postgres.StringColumn("name")
	ancestorsDepth := postgres.IntegerColumn("depth")
	ancestors := postgres.CTE("ancestors", ancestorsID, ancestorsParentID, ancestorsName, ancestorsDepth)

	anchor := postgres.SELECT(
		table.Files.ID,
		table.Files.ParentID,
		table.Files.Name,
		postgres.CAST(postgres.Int(0)).AS_INTEGER().AS("depth"),
	).FROM(table.Files).WHERE(table.Files.ID.EQ(postgres.UUID(fileID)))

	recursive := postgres.SELECT(
		table.Files.ID,
		table.Files.ParentID,
		table.Files.Name,
		postgres.CAST(ancestorsDepth.From(ancestors)).AS_INTEGER().ADD(postgres.Int(1)).AS("depth"),
	).FROM(
		table.Files.INNER_JOIN(ancestors, table.Files.ID.EQ(ancestorsParentID.From(ancestors))),
	)

	type pathSegment struct {
		Name  string
		Depth int64
	}

	stmt := postgres.WITH_RECURSIVE(
		ancestors.AS(anchor.UNION_ALL(recursive)),
	)(
		postgres.SELECT(
			ancestorsName.From(ancestors).AS("path_segment.name"),
			ancestorsDepth.From(ancestors).AS("path_segment.depth"),
		).FROM(ancestors).ORDER_BY(ancestorsDepth.From(ancestors).DESC()),
	)

	var segments []pathSegment
	if err := r.db.query(ctx, stmt, &segments); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if len(segments) == 0 {
		return "", ErrNotFound
	}

	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment.Name == "root" {
			continue
		}
		parts = append(parts, segment.Name)
	}

	if len(parts) == 0 {
		return "/", nil
	}

	return "/" + strings.Join(parts, "/"), nil
}

func (r *JetFileRepository) ListPendingForDeletion(ctx context.Context) ([]PendingFile, error) {
	stmt := table.Files.
		SELECT(table.Files.ID, table.Files.Parts, table.Files.ChannelID, table.Files.UserID).
		FROM(table.Files).
		WHERE(table.Files.Type.EQ(postgres.String("file")).AND(table.Files.Status.EQ(postgres.String("pending_deletion"))))

	var out []PendingFile
	if err := r.db.query(ctx, stmt, &out); err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return []PendingFile{}, nil
		}
		return nil, err
	}

	return out, nil
}

func (r *JetFileRepository) DeletePendingForDeletionByUser(ctx context.Context, userID int64) error {
	stmt := table.Files.DELETE().WHERE(
		table.Files.UserID.EQ(postgres.Int64(userID)).AND(table.Files.Status.EQ(postgres.String("pending_deletion"))),
	)

	return r.db.exec(ctx, stmt)
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
	_, err := r.DeleteBulkReturning(ctx, fileIDs, userID, targetStatus)
	return err
}

func (r *JetFileRepository) DeleteBulkReturning(ctx context.Context, fileIDs []uuid.UUID, userID int64, targetStatus string) ([]model.Files, error) {
	if len(fileIDs) == 0 {
		return nil, nil
	}

	idExprs := make([]postgres.Expression, 0, len(fileIDs))
	for _, id := range fileIDs {
		idExprs = append(idExprs, postgres.UUID(id))
	}

	subtreeID := postgres.StringColumn("id")
	subtree := postgres.CTE("subtree", subtreeID)
	projections := fileReadProjections(table.Files)

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
			WHERE(table.Files.ID.IN(subtree.SELECT(subtreeID.From(subtree)))).
			RETURNING(projections...),
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

	operation := strings.ToLower(params.Operation)
	query, err := r.buildFilesQuery(ctx, params, operation)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return []model.Files{}, nil
		}
		return nil, err
	}

	listQuery, _, err := filesquery.NewBuilder().Build(query)
	if err != nil {
		return nil, err
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

func (r *JetFileRepository) buildFilesQuery(ctx context.Context, params FileQueryParams, operation string) (filesquery.Query, error) {
	query := filesquery.Query{
		UserID:     params.UserID,
		Operation:  mapFileQueryOperation(operation),
		Status:     params.Status,
		Path:       params.Path,
		Name:       params.Name,
		Type:       params.Type,
		Categories: params.Category,
		Search: filesquery.SearchParams{
			Query:      params.Query,
			SearchType: mapFileQuerySearchType(params.SearchType),
			DeepSearch: params.DeepSearch,
		},
		Shared: params.Shared,
		Sort:   mapFileQuerySortField(params.Sort),
		Order:  mapFileQuerySortOrder(params.Order),
		Cursor: params.Cursor,
		Limit:  params.Limit,
	}

	if params.UpdatedAt != "" {
		dateFilters, err := filesquery.ParseDateFilters(params.UpdatedAt)
		if err != nil {
			return filesquery.Query{}, err
		}
		query.UpdatedAt = dateFilters
	}

	parentID, parentIsNil, err := r.resolveFilesQueryParentID(ctx, params, operation)
	if err != nil {
		return filesquery.Query{}, err
	}
	query.ParentID = parentID
	query.ParentIsNil = parentIsNil

	return query, nil
}

func (r *JetFileRepository) resolveFilesQueryParentID(ctx context.Context, params FileQueryParams, operation string) (*uuid.UUID, bool, error) {
	if params.ParentID != "" {
		if operation == "find" && params.ParentID == "nil" {
			return nil, true, nil
		}

		parentID, err := uuid.Parse(params.ParentID)
		if err != nil {
			return nil, false, err
		}
		return &parentID, false, nil
	}

	switch operation {
	case "list":
		if params.Path == "" {
			return nil, false, nil
		}
		id, err := r.ResolvePathID(ctx, params.Path, params.UserID)
		if err != nil {
			return nil, false, err
		}
		return id, false, nil
	case "find":
		if params.Path == "" {
			return nil, false, nil
		}
		id, err := r.ResolvePathID(ctx, params.Path, params.UserID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				if params.Query == "" {
					return nil, false, err
				}
				return nil, false, nil
			}
			return nil, false, err
		}
		return id, false, nil
	default:
		return nil, false, nil
	}
}

func mapFileQueryOperation(operation string) filesquery.Operation {
	if strings.EqualFold(operation, string(filesquery.OpList)) {
		return filesquery.OpList
	}
	return filesquery.OpFind
}

func mapFileQuerySearchType(searchType string) filesquery.SearchType {
	if strings.EqualFold(searchType, string(filesquery.SearchTypeRegex)) {
		return filesquery.SearchTypeRegex
	}
	if strings.EqualFold(searchType, string(filesquery.SearchTypeText)) {
		return filesquery.SearchTypeText
	}
	return filesquery.SearchTypeDefault
}

func mapFileQuerySortField(sort string) filesquery.SortField {
	switch strings.ToLower(sort) {
	case "name":
		return filesquery.SortFieldName
	case "size":
		return filesquery.SortFieldSize
	case "id":
		return filesquery.SortFieldID
	default:
		return filesquery.SortFieldUpdatedAt
	}
}

func mapFileQuerySortOrder(order string) filesquery.SortOrder {
	if strings.EqualFold(order, string(filesquery.SortOrderAsc)) {
		return filesquery.SortOrderAsc
	}
	return filesquery.SortOrderDesc
}
