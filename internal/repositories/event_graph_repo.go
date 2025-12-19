package repositories

import (
	"context"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type EventGraphRepository struct {
	driver neo4j.DriverWithContext
}

func NewEventGraphRepository(driver neo4j.DriverWithContext) *EventGraphRepository {
	return &EventGraphRepository{driver: driver}
}

// AddAttendee adds a relationship (:User)-[:GOING]->(:Event)
func (r *EventGraphRepository) AddAttendee(ctx context.Context, userID, eventID primitive.ObjectID) error {
	query := `
		MERGE (u:User {id: $userID})
		MERGE (e:Event {id: $eventID})
		MERGE (u)-[:GOING]->(e)
	`
	params := map[string]any{
		"userID":  userID.Hex(),
		"eventID": eventID.Hex(),
	}

	_, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	return err
}

// RemoveAttendee removes the relationship (:User)-[:GOING]->(:Event)
func (r *EventGraphRepository) RemoveAttendee(ctx context.Context, userID, eventID primitive.ObjectID) error {
	query := `
		MATCH (u:User {id: $userID})-[r:GOING]->(e:Event {id: $eventID})
		DELETE r
	`
	params := map[string]any{
		"userID":  userID.Hex(),
		"eventID": eventID.Hex(),
	}

	_, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	return err
}

// GetFriendsGoing returns IDs of friends who are going to the event
func (r *EventGraphRepository) GetFriendsGoing(ctx context.Context, userID, eventID primitive.ObjectID) ([]string, error) {
	// Find (Me)-[:FRIEND]-(Friend)-[:GOING]->(Event)
	query := `
		MATCH (me:User {id: $userID})-[:FRIEND]-(f:User)-[:GOING]->(e:Event {id: $eventID})
		RETURN f.id as friendID
	`
	params := map[string]any{
		"userID":  userID.Hex(),
		"eventID": eventID.Hex(),
	}

	result, err := neo4j.ExecuteQuery(ctx, r.driver, query, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		return nil, err
	}

	var friendIDs []string
	for _, record := range result.Records {
		if id, ok := record.Values[0].(string); ok {
			friendIDs = append(friendIDs, id)
		}
	}
	return friendIDs, nil
}
