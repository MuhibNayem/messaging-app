package graph

import (
	"context"
	"log"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type Neo4jClient struct {
	Driver neo4j.DriverWithContext
}

func NewNeo4jClient(uri, username, password string) (*Neo4jClient, error) {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return nil, err
	}

	// Verify connection
	ctx := context.Background()
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return nil, err
	}

	log.Println("Connected to Neo4j Graph DB")
	return &Neo4jClient{Driver: driver}, nil
}

func (c *Neo4jClient) Close(ctx context.Context) error {
	return c.Driver.Close(ctx)
}

// IsMemberOf checks if a user is a member of a group (O(1) graph pattern match)
func (c *Neo4jClient) IsMemberOf(ctx context.Context, userID, groupID string) (bool, error) {
	session := c.Driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx,
		`MATCH (u:User {id: $userID})-[:MEMBER_OF]->(g:Group {id: $groupID})
		 RETURN count(u) > 0 AS isMember`,
		map[string]interface{}{"userID": userID, "groupID": groupID})
	if err != nil {
		return false, err
	}

	if result.Next(ctx) {
		return result.Record().Values[0].(bool), nil
	}
	return false, nil
}

// AddMember creates a MEMBER_OF relationship (idempotent with MERGE)
func (c *Neo4jClient) AddMember(ctx context.Context, userID, groupID string) error {
	session := c.Driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.Run(ctx,
		`MERGE (u:User {id: $userID})
		 MERGE (g:Group {id: $groupID})
		 MERGE (u)-[:MEMBER_OF {joined_at: datetime()}]->(g)`,
		map[string]interface{}{"userID": userID, "groupID": groupID})
	return err
}

// RemoveMember deletes the MEMBER_OF relationship
func (c *Neo4jClient) RemoveMember(ctx context.Context, userID, groupID string) error {
	session := c.Driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.Run(ctx,
		`MATCH (u:User {id: $userID})-[r:MEMBER_OF]->(g:Group {id: $groupID})
		 DELETE r`,
		map[string]interface{}{"userID": userID, "groupID": groupID})
	return err
}

// GetGroupMembers returns all member IDs for a group
func (c *Neo4jClient) GetGroupMembers(ctx context.Context, groupID string) ([]string, error) {
	session := c.Driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx,
		`MATCH (u:User)-[:MEMBER_OF]->(g:Group {id: $groupID})
		 RETURN u.id AS userID`,
		map[string]interface{}{"groupID": groupID})
	if err != nil {
		return nil, err
	}

	var members []string
	for result.Next(ctx) {
		if id, ok := result.Record().Values[0].(string); ok {
			members = append(members, id)
		}
	}
	return members, nil
}
