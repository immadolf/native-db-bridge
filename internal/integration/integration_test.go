//go:build integration

package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestIntegrationSmoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	mysqlDB, err := sql.Open("mysql", "ndb:ndb@tcp(127.0.0.1:33067)/ndb_test?parseTime=true&charset=utf8mb4")
	if err != nil {
		t.Fatal(err)
	}
	defer mysqlDB.Close()
	var one int
	if err := mysqlDB.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("mysql SELECT 1: %v", err)
	}
	if one != 1 {
		t.Fatalf("mysql returned %d", one)
	}

	redisDB0 := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6389", DB: 0})
	defer redisDB0.Close()
	redisDB1 := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6389", DB: 1})
	defer redisDB1.Close()
	if err := redisDB0.Set(ctx, "namespace-key", "db0", time.Minute).Err(); err != nil {
		t.Fatalf("redis db0 set: %v", err)
	}
	if got, err := redisDB1.Exists(ctx, "namespace-key").Result(); err != nil || got != 0 {
		t.Fatalf("redis namespace isolation exists=%d err=%v", got, err)
	}

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://127.0.0.1:27027"))
	if err != nil {
		t.Fatalf("mongo connect: %v", err)
	}
	defer mongoClient.Disconnect(ctx)
	if err := mongoClient.Ping(ctx, nil); err != nil {
		t.Fatalf("mongo ping: %v", err)
	}
}
