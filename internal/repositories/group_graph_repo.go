package repositories

import (
	"context"
	"log"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type GroupGraphRepository struct {
	driver neo4j.DriverWithContext
}

func NewGroupGraphRepository(driver neo4j.DriverWithContext) *GroupGraphRepository {
	return &GroupGraphRepository{driver: driver}
}

// SyncGroup ensures a group node exists
func (r *GroupGraphRepository) SyncGroup(ctx context.Context, groupID primitive.ObjectID, name string) error {
	query := `MERGE (g:Group {id: $groupID}) SET g.name = $name`
	params := map[string]any{"groupID": groupID.Hex(), "name": name}
	_, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err == nil {
		log.Printf("[Neo4j] Synced Group %s", groupID.Hex())
	}
	return err
}

// AddMember creates (:User)-[:MEMBER_OF]->(:Group) relationship
func (r *GroupGraphRepository) AddMember(ctx context.Context, userID, groupID primitive.ObjectID) error {
	query := `
		MERGE (u:User {id: $userID})
		MERGE (g:Group {id: $groupID})
		MERGE (u)-[r:MEMBER_OF]->(g)
		ON CREATE SET r.joined_at = datetime()
	`
	params := map[string]any{
		"userID":  userID.Hex(),
		"groupID": groupID.Hex(),
	}
	_, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err == nil {
		log.Printf("[Neo4j] Added member %s to group %s", userID.Hex(), groupID.Hex())
	}
	return err
}

// RemoveMember deletes MEMBER_OF relationship
func (r *GroupGraphRepository) RemoveMember(ctx context.Context, userID, groupID primitive.ObjectID) error {
	query := `
		MATCH (u:User {id: $userID})-[r:MEMBER_OF]->(g:Group {id: $groupID})
		DELETE r
	`
	params := map[string]any{
		"userID":  userID.Hex(),
		"groupID": groupID.Hex(),
	}
	_, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err == nil {
		log.Printf("[Neo4j] Removed member %s from group %s", userID.Hex(), groupID.Hex())
	}
	return err
}

// IsMember checks if user is a member of a group (O(1) graph pattern match)
func (r *GroupGraphRepository) IsMember(ctx context.Context, userID, groupID primitive.ObjectID) (bool, error) {
	query := `
		MATCH (u:User {id: $userID})-[:MEMBER_OF]->(g:Group {id: $groupID})
		RETURN count(u) > 0 AS isMember
	`
	params := map[string]any{
		"userID":  userID.Hex(),
		"groupID": groupID.Hex(),
	}
	result, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		return false, err
	}
	if len(result.Records) > 0 {
		return result.Records[0].Values[0].(bool), nil
	}
	return false, nil
}

// GetMembers returns all member IDs for a group
func (r *GroupGraphRepository) GetMembers(ctx context.Context, groupID primitive.ObjectID) ([]string, error) {
	query := `MATCH (u:User)-[:MEMBER_OF]->(g:Group {id: $groupID}) RETURN u.id`
	params := map[string]any{"groupID": groupID.Hex()}

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

// SyncAllMembers syncs all members for a group (useful for initial sync from MongoDB)
func (r *GroupGraphRepository) SyncAllMembers(ctx context.Context, groupID primitive.ObjectID, memberIDs []primitive.ObjectID) error {
	for _, memberID := range memberIDs {
		if err := r.AddMember(ctx, memberID, groupID); err != nil {
			log.Printf("[Neo4j] Warning: Failed to sync member %s to group %s: %v", memberID.Hex(), groupID.Hex(), err)
		}
	}
	return nil
}
