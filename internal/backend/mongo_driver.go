package backend

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"native-db-bridge-mcp/internal/config"
	"native-db-bridge-mcp/internal/lifecycle"
)

// mongoResource wraps a *mongo.Client and the fixed database name
// to satisfy lifecycle.Resource.
type mongoResource struct {
	client *mongo.Client
	dbName string
}

func (r *mongoResource) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return r.client.Disconnect(ctx)
}

// MongoDriverBackend implements MongoBackend using the official
// MongoDB Go driver. Each datasource maps to a single mongo.Client
// with the database fixed to the datasource's default_database.
// Connections are managed lazily via lifecycle.Manager.
type MongoDriverBackend struct {
	cfg     config.Config
	manager *lifecycle.Manager[string]
}

// NewMongoDriverBackend creates a MongoDB backend that defers connection
// until the first operation. The constructor never opens a connection.
func NewMongoDriverBackend(cfg config.Config) *MongoDriverBackend {
	idleTTL := cfg.ConnectionLifecycle.Defaults.IdleTTL.Duration
	if cfg.ConnectionLifecycle.Mongo.IdleTTL.Duration > 0 {
		idleTTL = cfg.ConnectionLifecycle.Mongo.IdleTTL.Duration
	}
	if idleTTL == 0 {
		idleTTL = 10 * time.Minute
	}

	factory := func(ctx context.Context, datasource string) (lifecycle.Resource, error) {
		connCfg, dsCfg, err := findMongoConnectionAndDatasource(cfg, datasource)
		if err != nil {
			return nil, err
		}

		clientOpts := options.Client().ApplyURI(connCfg.URI)
		client, err := mongo.Connect(ctx, clientOpts)
		if err != nil {
			return nil, fmt.Errorf("mongo connect %s: %w", datasource, err)
		}

		if err := client.Ping(ctx, nil); err != nil {
			_ = client.Disconnect(ctx)
			return nil, fmt.Errorf("mongo ping %s: %w", datasource, err)
		}

		return &mongoResource{
			client: client,
			dbName: dsCfg.DefaultDatabase,
		}, nil
	}

	return &MongoDriverBackend{
		cfg:     cfg,
		manager: lifecycle.NewManager(idleTTL, factory),
	}
}

// Close shuts down the lifecycle manager and all managed connections.
func (b *MongoDriverBackend) Close() error {
	return b.manager.Close()
}

// Ping verifies connectivity to the given datasource.
func (b *MongoDriverBackend) Ping(ctx context.Context, datasource string) error {
	release, err := b.manager.Acquire(ctx, datasource)
	if err != nil {
		return err
	}
	release()
	return nil
}

// Find executes find or aggregate operations and returns matching documents.
func (b *MongoDriverBackend) Find(ctx context.Context, req MongoFindRequest) (MongoResult, error) {
	start := time.Now()

	release, err := b.manager.Acquire(ctx, req.Datasource)
	if err != nil {
		return MongoResult{}, err
	}
	defer release()

	res, err := b.getResource(req.Datasource)
	if err != nil {
		return MongoResult{}, err
	}

	coll := res.client.Database(res.dbName).Collection(req.Collection)

	limit := int64(req.Limit)
	if limit <= 0 {
		limit = int64(b.cfg.Server.MaxResultRows)
	}

	var documents []map[string]interface{}

	switch req.Operation {
	case "find":
		findOpts := options.Find().SetLimit(limit)
		if req.Sort != nil {
			findOpts.SetSort(req.Sort)
		}
		if req.Projection != nil {
			findOpts.SetProjection(req.Projection)
		}

		filter := bson.M{}
		if req.Filter != nil {
			filter = toBsonM(req.Filter)
		}

		cur, err := coll.Find(ctx, filter, findOpts)
		if err != nil {
			return MongoResult{}, fmt.Errorf("mongo find %s.%s: %w", req.Datasource, req.Collection, err)
		}
		defer cur.Close(ctx)

		if err := cur.All(ctx, &documents); err != nil {
			return MongoResult{}, fmt.Errorf("mongo decode %s.%s: %w", req.Datasource, req.Collection, err)
		}

	case "findOne":
		filter := bson.M{}
		if req.Filter != nil {
			filter = toBsonM(req.Filter)
		}

		findOneOpts := options.FindOne()
		if req.Projection != nil {
			findOneOpts.SetProjection(req.Projection)
		}

		var doc map[string]interface{}
		err := coll.FindOne(ctx, filter, findOneOpts).Decode(&doc)
		if err != nil && err != mongo.ErrNoDocuments {
			return MongoResult{}, fmt.Errorf("mongo findOne %s.%s: %w", req.Datasource, req.Collection, err)
		}
		if doc != nil {
			documents = append(documents, doc)
		}

	case "countDocuments":
		filter := bson.M{}
		if req.Filter != nil {
			filter = toBsonM(req.Filter)
		}

		count, err := coll.CountDocuments(ctx, filter)
		if err != nil {
			return MongoResult{}, fmt.Errorf("mongo count %s.%s: %w", req.Datasource, req.Collection, err)
		}
		documents = []map[string]interface{}{{"count": count}}

	case "distinct":
		if req.Filter == nil {
			return MongoResult{}, fmt.Errorf("mongo distinct requires a filter with 'field' key")
		}
		field, ok := req.Filter["field"].(string)
		if !ok {
			return MongoResult{}, fmt.Errorf("mongo distinct filter must contain string 'field'")
		}

		distinctFilter := bson.M{}
		if sub, ok := req.Filter["query"]; ok {
			if subMap, ok := sub.(map[string]interface{}); ok {
				distinctFilter = toBsonM(subMap)
			}
		}

		vals, err := coll.Distinct(ctx, field, distinctFilter)
		if err != nil {
			return MongoResult{}, fmt.Errorf("mongo distinct %s.%s: %w", req.Datasource, req.Collection, err)
		}
		documents = []map[string]interface{}{{"field": field, "values": vals}}

	case "aggregate":
		pipeline := make([]bson.M, 0, len(req.Pipeline))
		for _, stage := range req.Pipeline {
			if stageMap, ok := stage.(map[string]interface{}); ok {
				pipeline = append(pipeline, toBsonM(stageMap))
			}
		}

		aggOpts := options.Aggregate().SetAllowDiskUse(true)
		cur, err := coll.Aggregate(ctx, pipeline, aggOpts)
		if err != nil {
			return MongoResult{}, fmt.Errorf("mongo aggregate %s.%s: %w", req.Datasource, req.Collection, err)
		}
		defer cur.Close(ctx)

		if err := cur.All(ctx, &documents); err != nil {
			return MongoResult{}, fmt.Errorf("mongo decode aggregate %s.%s: %w", req.Datasource, req.Collection, err)
		}

	default:
		return MongoResult{}, fmt.Errorf("unsupported mongo operation: %s", req.Operation)
	}

	return MongoResult{
		Documents: documents,
		Count:     len(documents),
		Elapsed:   time.Since(start),
	}, nil
}

// Write executes insert, update, or delete operations.
func (b *MongoDriverBackend) Write(ctx context.Context, req MongoWriteRequest) (ExecResult, error) {
	start := time.Now()

	release, err := b.manager.Acquire(ctx, req.Datasource)
	if err != nil {
		return ExecResult{}, err
	}
	defer release()

	res, err := b.getResource(req.Datasource)
	if err != nil {
		return ExecResult{}, err
	}

	coll := res.client.Database(res.dbName).Collection(req.Collection)

	var affected int64

	switch req.Operation {
	case "insertOne":
		doc := toBsonM(req.Document)
		result, err := coll.InsertOne(ctx, doc)
		if err != nil {
			return ExecResult{}, fmt.Errorf("mongo insertOne %s.%s: %w", req.Datasource, req.Collection, err)
		}
		if result.InsertedID != nil {
			affected = 1
		}

	case "insertMany":
		docs := make([]interface{}, len(req.Documents))
		for i, d := range req.Documents {
			docs[i] = toBsonM(d)
		}
		result, err := coll.InsertMany(ctx, docs)
		if err != nil {
			return ExecResult{}, fmt.Errorf("mongo insertMany %s.%s: %w", req.Datasource, req.Collection, err)
		}
		affected = int64(len(result.InsertedIDs))

	case "updateOne":
		filter := toBsonM(req.Filter)
		update := toBsonM(req.Document)
		result, err := coll.UpdateOne(ctx, filter, update)
		if err != nil {
			return ExecResult{}, fmt.Errorf("mongo updateOne %s.%s: %w", req.Datasource, req.Collection, err)
		}
		affected = result.ModifiedCount

	case "updateMany":
		filter := toBsonM(req.Filter)
		update := toBsonM(req.Document)
		result, err := coll.UpdateMany(ctx, filter, update)
		if err != nil {
			return ExecResult{}, fmt.Errorf("mongo updateMany %s.%s: %w", req.Datasource, req.Collection, err)
		}
		affected = result.ModifiedCount

	case "deleteOne":
		filter := toBsonM(req.Filter)
		result, err := coll.DeleteOne(ctx, filter)
		if err != nil {
			return ExecResult{}, fmt.Errorf("mongo deleteOne %s.%s: %w", req.Datasource, req.Collection, err)
		}
		affected = result.DeletedCount

	case "deleteMany":
		filter := toBsonM(req.Filter)
		result, err := coll.DeleteMany(ctx, filter)
		if err != nil {
			return ExecResult{}, fmt.Errorf("mongo deleteMany %s.%s: %w", req.Datasource, req.Collection, err)
		}
		affected = result.DeletedCount

	default:
		return ExecResult{}, fmt.Errorf("unsupported mongo write operation: %s", req.Operation)
	}

	return ExecResult{
		AffectedCount: affected,
		Elapsed:       time.Since(start),
	}, nil
}

// ListDatabases returns the datasource's default_database in v1.
func (b *MongoDriverBackend) ListDatabases(_ context.Context, datasource string) ([]string, error) {
	for _, ds := range b.cfg.Datasources.Mongo {
		if ds.Name == datasource {
			return []string{ds.DefaultDatabase}, nil
		}
	}
	return nil, fmt.Errorf("datasource %q not found", datasource)
}

// ListCollections returns collection names matching the optional pattern.
func (b *MongoDriverBackend) ListCollections(ctx context.Context, datasource, pattern string) ([]MongoCollection, error) {
	release, err := b.manager.Acquire(ctx, datasource)
	if err != nil {
		return nil, err
	}
	defer release()

	res, err := b.getResource(datasource)
	if err != nil {
		return nil, err
	}

	db := res.client.Database(res.dbName)

	filter := bson.M{}
	if pattern != "" {
		filter["name"] = bson.M{"$regex": pattern}
	}

	names, err := db.ListCollectionNames(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("mongo list collections %s: %w", datasource, err)
	}

	collections := make([]MongoCollection, len(names))
	for i, name := range names {
		collections[i] = MongoCollection{Name: name, Type: "collection"}
	}

	return collections, nil
}

// DescribeCollection returns metadata and sample schema for a collection.
func (b *MongoDriverBackend) DescribeCollection(ctx context.Context, datasource, collection string) (MongoCollectionDescription, error) {
	release, err := b.manager.Acquire(ctx, datasource)
	if err != nil {
		return MongoCollectionDescription{}, err
	}
	defer release()

	res, err := b.getResource(datasource)
	if err != nil {
		return MongoCollectionDescription{}, err
	}

	db := res.client.Database(res.dbName)
	coll := db.Collection(collection)

	// Estimated count.
	count, err := coll.EstimatedDocumentCount(ctx)
	if err != nil {
		return MongoCollectionDescription{}, fmt.Errorf("mongo estimated count %s.%s: %w", datasource, collection, err)
	}

	// Indexes.
	idxCursor, err := coll.Indexes().List(ctx)
	if err != nil {
		return MongoCollectionDescription{}, fmt.Errorf("mongo list indexes %s.%s: %w", datasource, collection, err)
	}
	defer idxCursor.Close(ctx)

	var indexes []IndexInfo
	for idxCursor.Next(ctx) {
		var idx bson.M
		if err := idxCursor.Decode(&idx); err != nil {
			return MongoCollectionDescription{}, fmt.Errorf("mongo decode index %s.%s: %w", datasource, collection, err)
		}
		info := IndexInfo{
			Name: fmt.Sprintf("%v", idx["name"]),
		}
		if key, ok := idx["key"].(bson.M); ok {
			info.Key = make(map[string]interface{}, len(key))
			for k, v := range key {
				info.Key[k] = v
			}
		}
		if unique, ok := idx["unique"].(bool); ok {
			info.Unique = unique
		}
		indexes = append(indexes, info)
	}

	// Sample schema from a bounded sample query.
	sampleSchema := b.sampleSchema(ctx, coll)

	return MongoCollectionDescription{
		Collection:     collection,
		EstimatedCount: count,
		Indexes:        indexes,
		SampleSchema:   sampleSchema,
	}, nil
}

// sampleSchema runs a $sample aggregation and extracts field names/types.
func (b *MongoDriverBackend) sampleSchema(ctx context.Context, coll *mongo.Collection) map[string]interface{} {
	sampleCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{"$sample": bson.M{"size": 1}},
	}
	cur, err := coll.Aggregate(sampleCtx, pipeline)
	if err != nil {
		return nil
	}
	defer cur.Close(sampleCtx)

	var docs []bson.M
	if err := cur.All(sampleCtx, &docs); err != nil || len(docs) == 0 {
		return nil
	}

	schema := make(map[string]interface{})
	for k, v := range docs[0] {
		schema[k] = bsonTypeName(v)
	}
	return schema
}

// getResource returns the underlying mongo resource for a datasource.
func (b *MongoDriverBackend) getResource(datasource string) (*mongoResource, error) {
	res, ok := b.manager.Get(datasource)
	if !ok {
		return nil, fmt.Errorf("datasource %q not found in lifecycle manager", datasource)
	}
	mongoRes, ok := res.(*mongoResource)
	if !ok {
		return nil, fmt.Errorf("unexpected resource type for datasource %q", datasource)
	}
	return mongoRes, nil
}

// findMongoConnectionAndDatasource looks up the connection config and
// datasource config for a given datasource name.
func findMongoConnectionAndDatasource(cfg config.Config, datasource string) (config.MongoConnectionConfig, config.MongoDatasourceConfig, error) {
	var dsCfg config.MongoDatasourceConfig
	found := false
	for _, ds := range cfg.Datasources.Mongo {
		if ds.Name == datasource {
			dsCfg = ds
			found = true
			break
		}
	}
	if !found {
		return config.MongoConnectionConfig{}, config.MongoDatasourceConfig{}, fmt.Errorf("datasource %q not found", datasource)
	}

	for _, conn := range cfg.Connections.Mongo {
		if conn.Name == dsCfg.Connection {
			return conn, dsCfg, nil
		}
	}

	return config.MongoConnectionConfig{}, config.MongoDatasourceConfig{}, fmt.Errorf("connection %q not found for datasource %q", dsCfg.Connection, datasource)
}

// toBsonM converts a map[string]interface{} to bson.M recursively.
func toBsonM(m map[string]interface{}) bson.M {
	if m == nil {
		return bson.M{}
	}
	result := make(bson.M, len(m))
	for k, v := range m {
		switch typed := v.(type) {
		case map[string]interface{}:
			result[k] = toBsonM(typed)
		case []interface{}:
			result[k] = toBsonA(typed)
		default:
			result[k] = v
		}
	}
	return result
}

// toBsonA converts a []interface{} to bson.A recursively.
func toBsonA(a []interface{}) bson.A {
	result := make(bson.A, len(a))
	for i, v := range a {
		switch typed := v.(type) {
		case map[string]interface{}:
			result[i] = toBsonM(typed)
		case []interface{}:
			result[i] = toBsonA(typed)
		default:
			result[i] = v
		}
	}
	return result
}

// bsonTypeName returns a human-readable type name for a BSON value.
func bsonTypeName(v interface{}) string {
	switch v.(type) {
	case string:
		return "string"
	case int32:
		return "int32"
	case int64:
		return "int64"
	case float64:
		return "double"
	case bool:
		return "bool"
	case bson.M, map[string]interface{}:
		return "object"
	case bson.A, []interface{}:
		return "array"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}
