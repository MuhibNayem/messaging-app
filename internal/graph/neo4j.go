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
