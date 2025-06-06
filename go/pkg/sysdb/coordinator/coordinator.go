package coordinator

import (
	"context"

	"github.com/chroma-core/chroma/go/pkg/common"
	"github.com/chroma-core/chroma/go/pkg/proto/coordinatorpb"
	"github.com/chroma-core/chroma/go/pkg/sysdb/coordinator/model"
	"github.com/chroma-core/chroma/go/pkg/sysdb/metastore/db/dao"
	"github.com/chroma-core/chroma/go/pkg/sysdb/metastore/db/dbcore"
	"github.com/chroma-core/chroma/go/pkg/sysdb/metastore/db/dbmodel"
	s3metastore "github.com/chroma-core/chroma/go/pkg/sysdb/metastore/s3"
	"github.com/chroma-core/chroma/go/pkg/types"
	"github.com/pingcap/log"
	"go.uber.org/zap"
)

// DeleteMode represents whether to perform a soft or hard delete
type DeleteMode int

const (
	// SoftDelete marks records as deleted but keeps them in the database
	SoftDelete DeleteMode = iota
	// HardDelete permanently removes records from the database
	HardDelete
)

// Coordinator is the top level component.
// Currently, it only has the system catalog related APIs and will be extended to
// support other functionalities such as membership managed and propagation.
type Coordinator struct {
	ctx         context.Context
	catalog     Catalog
	deleteMode  DeleteMode
	objectStore *s3metastore.S3MetaStore
}

func NewCoordinator(ctx context.Context, deleteMode DeleteMode, objectStore *s3metastore.S3MetaStore, versionFileEnabled bool) (*Coordinator, error) {
	s := &Coordinator{
		ctx:         ctx,
		deleteMode:  deleteMode,
		objectStore: objectStore,
	}

	// catalog
	txnImpl := dbcore.NewTxImpl()
	metaDomain := dao.NewMetaDomain()
	s.catalog = *NewTableCatalog(txnImpl, metaDomain, s.objectStore, versionFileEnabled)
	return s, nil
}

func (s *Coordinator) ResetState(ctx context.Context) error {
	return s.catalog.ResetState(ctx)
}

func (s *Coordinator) CreateDatabase(ctx context.Context, createDatabase *model.CreateDatabase) (*model.Database, error) {
	database, err := s.catalog.CreateDatabase(ctx, createDatabase, createDatabase.Ts)
	if err != nil {
		return nil, err
	}
	return database, nil
}

func (s *Coordinator) GetDatabase(ctx context.Context, getDatabase *model.GetDatabase) (*model.Database, error) {
	database, err := s.catalog.GetDatabases(ctx, getDatabase, getDatabase.Ts)
	if err != nil {
		return nil, err
	}
	return database, nil
}

func (s *Coordinator) ListDatabases(ctx context.Context, listDatabases *model.ListDatabases) ([]*model.Database, error) {
	databases, err := s.catalog.ListDatabases(ctx, listDatabases, listDatabases.Ts)
	if err != nil {
		return nil, err
	}
	return databases, nil
}

func (s *Coordinator) DeleteDatabase(ctx context.Context, deleteDatabase *model.DeleteDatabase) error {
	return s.catalog.DeleteDatabase(ctx, deleteDatabase)
}

func (s *Coordinator) CreateTenant(ctx context.Context, createTenant *model.CreateTenant) (*model.Tenant, error) {
	tenant, err := s.catalog.CreateTenant(ctx, createTenant, createTenant.Ts)
	if err != nil {
		return nil, err
	}
	return tenant, nil
}

func (s *Coordinator) GetTenant(ctx context.Context, getTenant *model.GetTenant) (*model.Tenant, error) {
	tenant, err := s.catalog.GetTenants(ctx, getTenant, getTenant.Ts)
	if err != nil {
		return nil, err
	}
	return tenant, nil
}

func (s *Coordinator) CreateCollectionAndSegments(ctx context.Context, createCollection *model.CreateCollection, createSegments []*model.CreateSegment) (*model.Collection, bool, error) {
	collection, created, err := s.catalog.CreateCollectionAndSegments(ctx, createCollection, createSegments, createCollection.Ts)
	if err != nil {
		return nil, false, err
	}
	return collection, created, nil
}

func (s *Coordinator) CreateCollection(ctx context.Context, createCollection *model.CreateCollection) (*model.Collection, bool, error) {
	log.Info("create collection", zap.Any("createCollection", createCollection))
	collection, created, err := s.catalog.CreateCollection(ctx, createCollection, createCollection.Ts)
	if err != nil {
		return nil, false, err
	}
	return collection, created, nil
}

func (s *Coordinator) GetCollection(ctx context.Context, collectionID types.UniqueID, collectionName *string, tenantID string, databaseName string) (*model.Collection, error) {
	return s.catalog.GetCollection(ctx, collectionID, collectionName, tenantID, databaseName)
}

func (s *Coordinator) GetCollections(ctx context.Context, collectionID types.UniqueID, collectionName *string, tenantID string, databaseName string, limit *int32, offset *int32) ([]*model.Collection, error) {
	return s.catalog.GetCollections(ctx, collectionID, collectionName, tenantID, databaseName, limit, offset)
}

func (s *Coordinator) CountCollections(ctx context.Context, tenantID string, databaseName *string) (uint64, error) {
	return s.catalog.CountCollections(ctx, tenantID, databaseName)
}

func (s *Coordinator) GetCollectionSize(ctx context.Context, collectionID types.UniqueID) (uint64, error) {
	return s.catalog.GetCollectionSize(ctx, collectionID)
}

func (s *Coordinator) GetCollectionWithSegments(ctx context.Context, collectionID types.UniqueID) (*model.Collection, []*model.Segment, error) {
	return s.catalog.GetCollectionWithSegments(ctx, collectionID)
}

func (s *Coordinator) CheckCollection(ctx context.Context, collectionID types.UniqueID) (bool, error) {
	return s.catalog.CheckCollection(ctx, collectionID)
}

func (s *Coordinator) GetSoftDeletedCollections(ctx context.Context, collectionID *string, tenantID string, databaseName string, limit int32) ([]*model.Collection, error) {
	return s.catalog.GetSoftDeletedCollections(ctx, collectionID, tenantID, databaseName, limit)
}

func (s *Coordinator) DeleteCollection(ctx context.Context, deleteCollection *model.DeleteCollection) error {
	if s.deleteMode == SoftDelete {
		return s.catalog.DeleteCollection(ctx, deleteCollection, true)
	}
	return s.catalog.DeleteCollection(ctx, deleteCollection, false)
}

func (s *Coordinator) CleanupSoftDeletedCollection(ctx context.Context, deleteCollection *model.DeleteCollection) error {
	return s.catalog.DeleteCollection(ctx, deleteCollection, false)
}

func (s *Coordinator) UpdateCollection(ctx context.Context, collection *model.UpdateCollection) (*model.Collection, error) {
	return s.catalog.UpdateCollection(ctx, collection, collection.Ts)
}

func (s *Coordinator) ForkCollection(ctx context.Context, forkCollection *model.ForkCollection) (*model.Collection, []*model.Segment, error) {
	return s.catalog.ForkCollection(ctx, forkCollection)
}

func (s *Coordinator) CreateSegment(ctx context.Context, segment *model.CreateSegment) error {
	if err := verifyCreateSegment(segment); err != nil {
		return err
	}
	_, err := s.catalog.CreateSegment(ctx, segment, segment.Ts)
	if err != nil {
		return err
	}
	return nil
}

func (s *Coordinator) GetSegments(ctx context.Context, segmentID types.UniqueID, segmentType *string, scope *string, collectionID types.UniqueID) ([]*model.Segment, error) {
	return s.catalog.GetSegments(ctx, segmentID, segmentType, scope, collectionID)
}

// DeleteSegment is a no-op.
// Segments are deleted as part of atomic delete of collection.
// Keeping this API so that older clients continue to work, since older clients will issue DeleteSegment
// after a DeleteCollection.
func (s *Coordinator) DeleteSegment(ctx context.Context, segmentID types.UniqueID, collectionID types.UniqueID) error {
	return s.catalog.DeleteSegment(ctx, segmentID, collectionID)
}

func (s *Coordinator) UpdateSegment(ctx context.Context, updateSegment *model.UpdateSegment) (*model.Segment, error) {
	segment, err := s.catalog.UpdateSegment(ctx, updateSegment, updateSegment.Ts)
	if err != nil {
		return nil, err
	}
	return segment, nil
}

func verifyCollectionMetadata(metadata *model.CollectionMetadata[model.CollectionMetadataValueType]) error {
	if metadata == nil {
		return nil
	}
	for _, value := range metadata.Metadata {
		switch (value).(type) {
		case *model.CollectionMetadataValueStringType:
		case *model.CollectionMetadataValueInt64Type:
		case *model.CollectionMetadataValueFloat64Type:
		default:
			return common.ErrUnknownCollectionMetadataType
		}
	}
	return nil
}

func verifyCreateSegment(segment *model.CreateSegment) error {
	if err := verifySegmentMetadata(segment.Metadata); err != nil {
		return err
	}
	return nil
}

func verifySegmentMetadata(metadata *model.SegmentMetadata[model.SegmentMetadataValueType]) error {
	if metadata == nil {
		return nil
	}
	for _, value := range metadata.Metadata {
		switch (value).(type) {
		case *model.SegmentMetadataValueStringType:
		case *model.SegmentMetadataValueInt64Type:
		case *model.SegmentMetadataValueFloat64Type:
		default:
			return common.ErrUnknownSegmentMetadataType
		}
	}
	return nil
}

func (s *Coordinator) SetTenantLastCompactionTime(ctx context.Context, tenantID string, lastCompactionTime int64) error {
	return s.catalog.SetTenantLastCompactionTime(ctx, tenantID, lastCompactionTime)
}

func (s *Coordinator) GetTenantsLastCompactionTime(ctx context.Context, tenantIDs []string) ([]*dbmodel.Tenant, error) {
	return s.catalog.GetTenantsLastCompactionTime(ctx, tenantIDs)
}

func (s *Coordinator) FlushCollectionCompaction(ctx context.Context, flushCollectionCompaction *model.FlushCollectionCompaction) (*model.FlushCollectionInfo, error) {
	return s.catalog.FlushCollectionCompaction(ctx, flushCollectionCompaction)
}

func (s *Coordinator) ListCollectionsToGc(ctx context.Context, cutoffTimeSecs *uint64, limit *uint64) ([]*model.CollectionToGc, error) {
	return s.catalog.ListCollectionsToGc(ctx, cutoffTimeSecs, limit)
}

func (s *Coordinator) ListCollectionVersions(ctx context.Context, collectionID types.UniqueID, tenantID string, maxCount *int64, versionsBefore *int64, versionsAtOrAfter *int64, includeMarkedForDeletion bool) ([]*coordinatorpb.CollectionVersionInfo, error) {
	return s.catalog.ListCollectionVersions(ctx, collectionID, tenantID, maxCount, versionsBefore, versionsAtOrAfter, includeMarkedForDeletion)
}

func (s *Coordinator) MarkVersionForDeletion(ctx context.Context, req *coordinatorpb.MarkVersionForDeletionRequest) (*coordinatorpb.MarkVersionForDeletionResponse, error) {
	return s.catalog.MarkVersionForDeletion(ctx, req)
}

func (s *Coordinator) DeleteCollectionVersion(ctx context.Context, req *coordinatorpb.DeleteCollectionVersionRequest) (*coordinatorpb.DeleteCollectionVersionResponse, error) {
	return s.catalog.DeleteCollectionVersion(ctx, req)
}

// SetDeleteMode sets the delete mode for testing
func (c *Coordinator) SetDeleteMode(mode DeleteMode) {
	c.deleteMode = mode
}
