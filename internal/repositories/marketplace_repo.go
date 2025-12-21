package repositories

import (
	"context"
	"errors"
	"log"
	"messaging-app/internal/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MarketplaceRepository struct {
	db                 *mongo.Database
	productCollection  *mongo.Collection
	categoryCollection *mongo.Collection
	messageCollection  *mongo.Collection
}

func NewMarketplaceRepository(db *mongo.Database) *MarketplaceRepository {
	productCollection := db.Collection("products")
	categoryCollection := db.Collection("categories")
	messageCollection := db.Collection("messages") // Need access for aggregating conversations

	// Create Indexes for Products
	productIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "category_id", Value: 1},
				{Key: "status", Value: 1},
				{Key: "created_at", Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: "title", Value: "text"},
				{Key: "description", Value: "text"},
				{Key: "tags", Value: "text"},
			},
			Options: options.Index().SetName("product_text_index"),
		},
		{
			Keys: bson.D{{Key: "seller_id", Value: 1}},
		},
		{
			// Geospatial index for future location-based search
			Keys: bson.D{{Key: "coordinates", Value: "2dsphere"}},
		},
	}
	_, err := productCollection.Indexes().CreateMany(context.Background(), productIndexes)
	if err != nil {
		log.Printf("Failed to create product indexes: %v", err)
	}

	// Create Indexes for Categories
	categoryIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "slug", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "order", Value: 1}},
		},
	}
	_, err = categoryCollection.Indexes().CreateMany(context.Background(), categoryIndexes)
	if err != nil {
		log.Printf("Failed to create category indexes: %v", err)
	}

	return &MarketplaceRepository{
		db:                 db,
		productCollection:  productCollection,
		categoryCollection: categoryCollection,
		messageCollection:  messageCollection,
	}
}

// --- Category Methods ---

func (r *MarketplaceRepository) EnsureCategory(ctx context.Context, category *models.Category) error {
	opts := options.Update().SetUpsert(true)
	filter := bson.M{"slug": category.Slug}
	update := bson.M{
		"$set": bson.M{
			"name":       category.Name,
			"icon":       category.Icon,
			"order":      category.Order,
			"created_at": category.CreatedAt, // Only effective on insert if we used $setOnInsert, but here we update everything to sync seed changes
		},
	}
	_, err := r.categoryCollection.UpdateOne(ctx, filter, update, opts)
	return err
}

func (r *MarketplaceRepository) GetCategories(ctx context.Context) ([]models.Category, error) {
	opts := options.Find().SetSort(bson.D{{Key: "order", Value: 1}})
	cursor, err := r.categoryCollection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	categories := []models.Category{}
	if err = cursor.All(ctx, &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

// --- Product Methods ---

func (r *MarketplaceRepository) CreateProduct(ctx context.Context, product *models.Product) (*models.Product, error) {
	product.CreatedAt = time.Now()
	product.UpdatedAt = time.Now()
	product.Views = 0
	product.SavedBy = []primitive.ObjectID{}

	res, err := r.productCollection.InsertOne(ctx, product)
	if err != nil {
		return nil, err
	}
	product.ID = res.InsertedID.(primitive.ObjectID)
	return product, nil
}

func (r *MarketplaceRepository) GetProductByID(ctx context.Context, id primitive.ObjectID) (*models.Product, error) {
	var product models.Product
	err := r.productCollection.FindOne(ctx, bson.M{"_id": id}).Decode(&product)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.New("product not found")
		}
		return nil, err
	}
	return &product, nil
}

func (r *MarketplaceRepository) UpdateProduct(ctx context.Context, id primitive.ObjectID, update bson.M) (*models.Product, error) {
	update["updated_at"] = time.Now()

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var updatedProduct models.Product
	err := r.productCollection.FindOneAndUpdate(ctx, bson.M{"_id": id}, bson.M{"$set": update}, opts).Decode(&updatedProduct)
	if err != nil {
		return nil, err
	}
	return &updatedProduct, nil
}

func (r *MarketplaceRepository) DeleteProduct(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.productCollection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *MarketplaceRepository) IncrementViews(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.productCollection.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$inc": bson.M{"views": 1}})
	return err
}

func (r *MarketplaceRepository) ListProducts(ctx context.Context, filter models.ProductFilter) ([]models.ProductResponse, int64, error) {
	// Base match filter
	matchStage := bson.M{
		"status": models.ProductStatusAvailable, // Default to available
	}

	if filter.CategoryID != "" {
		catID, err := primitive.ObjectIDFromHex(filter.CategoryID)
		if err == nil {
			matchStage["category_id"] = catID
		}
	}

	if filter.Query != "" {
		matchStage["$text"] = bson.M{"$search": filter.Query}
	}

	if filter.MinPrice != nil || filter.MaxPrice != nil {
		priceFilter := bson.M{}
		if filter.MinPrice != nil {
			priceFilter["$gte"] = *filter.MinPrice
		}
		if filter.MaxPrice != nil {
			priceFilter["$lte"] = *filter.MaxPrice
		}
		matchStage["price"] = priceFilter
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: matchStage}},
	}

	// Calculate total count before pagination
	// Optimization: For highly scalable systems, count in a separate efficient query or estimate.
	// For now, we will use a separate CountDocuments query for accuracy.
	total, err := r.productCollection.CountDocuments(ctx, matchStage)
	if err != nil {
		return nil, 0, err
	}

	// Lookup Seller
	pipeline = append(pipeline,
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "seller_id",
			"foreignField": "_id",
			"as":           "seller_info",
		}}},
		bson.D{{Key: "$unwind", Value: bson.M{"path": "$seller_info", "preserveNullAndEmptyArrays": true}}},
	)

	// Lookup Category
	pipeline = append(pipeline,
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "categories",
			"localField":   "category_id",
			"foreignField": "_id",
			"as":           "category_info",
		}}},
		bson.D{{Key: "$unwind", Value: bson.M{"path": "$category_info", "preserveNullAndEmptyArrays": true}}},
	)

	// Sorting
	sortStage := bson.M{"created_at": -1} // Default Newest
	if filter.SortBy == "price_asc" {
		sortStage = bson.M{"price": 1}
	} else if filter.SortBy == "price_desc" {
		sortStage = bson.M{"price": -1}
	}
	pipeline = append(pipeline, bson.D{{Key: "$sort", Value: sortStage}})

	// Pagination
	pipeline = append(pipeline,
		bson.D{{Key: "$skip", Value: (filter.Page - 1) * filter.Limit}},
		bson.D{{Key: "$limit", Value: filter.Limit}},
	)

	// Projection
	pipeline = append(pipeline, bson.D{{Key: "$project", Value: bson.M{
		"_id":         1, // Keep original _id for product
		"title":       1,
		"description": 1,
		"price":       1,
		"currency":    1,
		"images":      1,
		"location":    1,
		"status":      1,
		"tags":        1,
		"views":       1,
		"created_at":  1,
		"seller": bson.M{
			"_id":       "$seller_info._id",
			"username":  "$seller_info.username",
			"full_name": "$seller_info.full_name",
			"avatar":    "$seller_info.avatar",
		},
		"category": bson.M{
			"_id":  "$category_info._id",
			"name": "$category_info.name",
			"slug": "$category_info.slug",
			"icon": "$category_info.icon",
		},
	}}})

	cursor, err := r.productCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	products := []models.ProductResponse{}
	if err = cursor.All(ctx, &products); err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

// --- Marketplace Messaging ---

// GetMarketplaceConversations aggregates messages to find conversations related to products.
// It groups by (other_user) to create ONE conversation per buyer-seller pair.
func (r *MarketplaceRepository) GetMarketplaceConversations(ctx context.Context, userID primitive.ObjectID) ([]models.ConversationSummary, error) {
	// Pipeline to find messages where the user is sender or receiver AND is_marketplace is true
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{
			"is_marketplace": true, // Use the robust is_marketplace flag
			"$or": []bson.M{
				{"sender_id": userID},
				{"receiver_id": userID},
			},
		}}},
		// Sort by created_at desc to get latest first
		bson.D{{Key: "$sort", Value: bson.M{"created_at": -1}}},
		// Group by the *Other* User ID only - ONE conversation per buyer-seller pair
		bson.D{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"$cond": bson.A{
					bson.M{"$eq": bson.A{"$sender_id", userID}},
					"$receiver_id",
					"$sender_id",
				},
			},
			"last_message": bson.M{"$first": "$$ROOT"},
			"unread_count": bson.M{
				"$sum": bson.M{
					"$cond": bson.A{
						bson.M{"$and": bson.A{
							bson.M{"$eq": bson.A{"$receiver_id", userID}},
							bson.M{"$not": bson.M{"$in": bson.A{userID, "$seen_by"}}},
						}},
						1,
						0,
					},
				},
			},
		}}},
		// Lookup Other User Info
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "_id",
			"foreignField": "_id",
			"as":           "other_user_info",
		}}},
		bson.D{{Key: "$unwind", Value: "$other_user_info"}}, // Strict unwind (exclude if user not found)
		// Project to Summary
		bson.D{{Key: "$project", Value: bson.M{
			"_id":                       "$other_user_info._id",
			"name":                      "$other_user_info.username",
			"avatar":                    "$other_user_info.avatar",
			"is_group":                  bson.M{"$literal": false},
			"last_message_content":      "$last_message.content",
			"last_message_timestamp":    "$last_message.created_at",
			"last_message_sender_id":    "$last_message.sender_id",
			"unread_count":              "$unread_count",
			"last_message_is_encrypted": "$last_message.is_encrypted",
		}}},
		bson.D{{Key: "$sort", Value: bson.M{"last_message_timestamp": -1}}},
	}

	cursor, err := r.messageCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var summaries []models.ConversationSummary = []models.ConversationSummary{} // Init empty
	if err = cursor.All(ctx, &summaries); err != nil {
		return nil, err
	}

	return summaries, nil
}
