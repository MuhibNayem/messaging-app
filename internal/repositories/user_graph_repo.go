package repositories

import (
	"context"
	"log"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type UserGraphRepository struct {
	driver neo4j.DriverWithContext
}

func NewUserGraphRepository(driver neo4j.DriverWithContext) *UserGraphRepository {
	return &UserGraphRepository{driver: driver}
}

// SyncUser ensures a user node exists
func (r *UserGraphRepository) SyncUser(ctx context.Context, userID primitive.ObjectID) error {
	query := `MERGE (u:User {id: $userID})`
	params := map[string]any{"userID": userID.Hex()}
	_, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err == nil {
		log.Printf("[Neo4j] Synced User %s", userID.Hex())
	}
	return err
}

// SendRequest creates (:User)-[:REQUESTED]->(:User)
func (r *UserGraphRepository) SendRequest(ctx context.Context, from, to primitive.ObjectID) error {
	query := `
		MERGE (u1:User {id: $from})
		MERGE (u2:User {id: $to})
		MERGE (u1)-[r:REQUESTED]->(u2) // Directional
		ON CREATE SET r.created_at = datetime()
	`
	params := map[string]any{
		"from": from.Hex(),
		"to":   to.Hex(),
	}
	_, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err == nil {
		log.Printf("[Neo4j] Sent Request from %s to %s", from.Hex(), to.Hex())
	}
	return err
}

// AcceptRequest creates FRIEND relationship and deletes REQUESTED
func (r *UserGraphRepository) AcceptRequest(ctx context.Context, from, to primitive.ObjectID) error {
	query := `
		MATCH (u1:User {id: $from})
		MATCH (u2:User {id: $to})
		OPTIONAL MATCH (u1)-[r:REQUESTED]-(u2) // Match request in either direction just in case
		DELETE r
		MERGE (u1)-[f:FRIEND]-(u2) // Undirected/Bi-directional semantics usually modeled as undirected or double directed? Neo4j is directed but we treat FRIEND as mutual.
		ON CREATE SET f.since = datetime()
	`
	params := map[string]any{
		"from": from.Hex(),
		"to":   to.Hex(),
	}
	_, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	return err
}

// RejectRequest deletes REQUESTED relationship
func (r *UserGraphRepository) RejectRequest(ctx context.Context, from, to primitive.ObjectID) error {
	query := `
		MATCH (u1:User {id: $from})-[r:REQUESTED]-(u2:User {id: $to})
		DELETE r
	`
	params := map[string]any{
		"from": from.Hex(),
		"to":   to.Hex(),
	}
	_, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	return err
}

// Unfriend deletes FRIEND relationship
func (r *UserGraphRepository) Unfriend(ctx context.Context, user1, user2 primitive.ObjectID) error {
	query := `
		MATCH (u1:User {id: $user1})-[r:FRIEND]-(u2:User {id: $user2})
		DELETE r
	`
	params := map[string]any{
		"user1": user1.Hex(),
		"user2": user2.Hex(),
	}
	_, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	return err
}

// BlockUser creates BLOCKED relationship and deletes any other relations
func (r *UserGraphRepository) BlockUser(ctx context.Context, blocker, blocked primitive.ObjectID) error {
	query := `
		MERGE (u1:User {id: $blocker})
		MERGE (u2:User {id: $blocked})
		MERGE (u1)-[b:BLOCKED]->(u2)
		ON CREATE SET b.created_at = datetime()
		WITH u1, u2
		OPTIONAL MATCH (u1)-[r:FRIEND|REQUESTED]-(u2)
		DELETE r
	`
	params := map[string]any{
		"blocker": blocker.Hex(),
		"blocked": blocked.Hex(),
	}
	_, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	return err
}

// UnblockUser removes BLOCKED relationship
func (r *UserGraphRepository) UnblockUser(ctx context.Context, blocker, blocked primitive.ObjectID) error {
	query := `
		MATCH (u1:User {id: $blocker})-[r:BLOCKED]->(u2:User {id: $blocked})
		DELETE r
	`
	params := map[string]any{
		"blocker": blocker.Hex(),
		"blocked": blocked.Hex(),
	}
	_, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	return err
}

// CheckFriendshipStatus checks returning: areFriends, requestSent, requestReceived, blockedByMe, blockedByOther
func (r *UserGraphRepository) CheckFriendshipStatus(ctx context.Context, me, other primitive.ObjectID) (bool, bool, bool, bool, bool, error) {
	query := `
		MATCH (me:User {id: $me}), (other:User {id: $other})
		RETURN 
			exists((me)-[:FRIEND]-(other)) as areFriends,
			exists((me)-[:REQUESTED]->(other)) as requestSent,
			exists((other)-[:REQUESTED]->(me)) as requestReceived,
			exists((me)-[:BLOCKED]->(other)) as blockedByMe,
			exists((other)-[:BLOCKED]->(me)) as blockedByOther
	`
	params := map[string]any{
		"me":    me.Hex(),
		"other": other.Hex(),
	}
	result, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		return false, false, false, false, false, err
	}
	if len(result.Records) == 0 {
		return false, false, false, false, false, nil
	}
	rec := result.Records[0]
	return rec.Values[0].(bool), rec.Values[1].(bool), rec.Values[2].(bool), rec.Values[3].(bool), rec.Values[4].(bool), nil
}

// AddFriendship creates (:User)-[:FRIEND]-(:User) (Legacy/Seed helper)
func (r *UserGraphRepository) AddFriendship(ctx context.Context, user1, user2 primitive.ObjectID) error {
	return r.AcceptRequest(ctx, user1, user2)
}

// GetFriendIDs using Graph
func (r *UserGraphRepository) GetFriendIDs(ctx context.Context, userID primitive.ObjectID) ([]string, error) {
	query := `MATCH (u:User {id: $userID})-[:FRIEND]-(f:User) RETURN f.id`
	params := map[string]any{"userID": userID.Hex()}

	result, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, rec := range result.Records {
		if id, ok := rec.Values[0].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
