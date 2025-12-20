package repositories

import (
	"context"
	"fmt"
	"time"

	"messaging-app/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type FeedRepository struct {
	postsCollection      *mongo.Collection
	commentsCollection   *mongo.Collection
	repliesCollection    *mongo.Collection
	reactionsCollection  *mongo.Collection
	albumsCollection     *mongo.Collection
	albumMediaCollection *mongo.Collection
}

func NewFeedRepository(db *mongo.Database) *FeedRepository {
	// ... (indexes for other collections)

	// albums indexes
	_, err := db.Collection("albums").Indexes().CreateMany(
		context.Background(),
		[]mongo.IndexModel{
			{Keys: bson.D{{Key: "user_id", Value: 1}}, Options: options.Index()},
			{Keys: bson.D{{Key: "type", Value: 1}}, Options: options.Index()},
			{Keys: bson.D{{Key: "created_at", Value: -1}}, Options: options.Index()},
		},
	)
	if err != nil {
		panic("Failed to create album indexes: " + err.Error())
	}

	// album_media indexes
	_, err = db.Collection("album_media").Indexes().CreateMany(
		context.Background(),
		[]mongo.IndexModel{
			{Keys: bson.D{{Key: "album_id", Value: 1}}, Options: options.Index()},
			{Keys: bson.D{{Key: "created_at", Value: -1}}, Options: options.Index()},
		},
	)
	if err != nil {
		panic("Failed to create album_media indexes: " + err.Error())
	}

	return &FeedRepository{
		postsCollection:      db.Collection("posts"),
		commentsCollection:   db.Collection("comments"),
		repliesCollection:    db.Collection("replies"),
		reactionsCollection:  db.Collection("reactions"),
		albumsCollection:     db.Collection("albums"),
		albumMediaCollection: db.Collection("album_media"),
	}
}

// ... (Posts methods)

// ----------------------------- Albums ----------------_____________

func (r *FeedRepository) CreateAlbum(ctx context.Context, album *models.Album) (*models.Album, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	now := time.Now()
	album.CreatedAt = now
	album.UpdatedAt = now

	res, err := r.albumsCollection.InsertOne(ctx, album)
	if err != nil {
		return nil, err
	}
	album.ID = res.InsertedID.(primitive.ObjectID)
	return album, nil
}

func (r *FeedRepository) GetAlbumByID(ctx context.Context, albumID primitive.ObjectID) (*models.Album, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var album models.Album
	// Media is no longer embedded, just fetching metadata
	if err := r.albumsCollection.FindOne(ctx, bson.M{"_id": albumID}).Decode(&album); err != nil {
		return nil, err
	}
	return &album, nil
}

func (r *FeedRepository) GetAlbumByType(ctx context.Context, userID primitive.ObjectID, albumType models.AlbumType) (*models.Album, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var album models.Album
	if err := r.albumsCollection.FindOne(ctx, bson.M{"user_id": userID, "type": albumType}).Decode(&album); err != nil {
		return nil, err
	}
	return &album, nil
}

func (r *FeedRepository) ListAlbums(ctx context.Context, userID primitive.ObjectID, limit, offset int64) ([]models.Album, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(limit)
	}
	if offset > 0 {
		opts.SetSkip(offset)
	}

	cursor, err := r.albumsCollection.Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var albums []models.Album
	if err := cursor.All(ctx, &albums); err != nil {
		return nil, err
	}
	if albums == nil {
		albums = []models.Album{}
	}
	return albums, nil
}

func (r *FeedRepository) AddMediaToAlbum(ctx context.Context, media []models.AlbumMedia) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if len(media) == 0 {
		return nil
	}

	// Convert []models.AlbumMedia to []interface{} for InsertMany
	docs := make([]interface{}, len(media))
	for i, v := range media {
		docs[i] = v
	}

	_, err := r.albumMediaCollection.InsertMany(ctx, docs)
	if err != nil {
		return err
	}

	// Update the album's updated_at timestamp
	_, _ = r.albumsCollection.UpdateOne(
		ctx,
		bson.M{"_id": media[0].AlbumID},
		bson.M{"$set": bson.M{"updated_at": time.Now()}},
	)

	return nil
}

func (r *FeedRepository) GetAlbumMedia(ctx context.Context, albumID primitive.ObjectID, limit, offset int64, mediaType string) ([]models.AlbumMedia, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	filter := bson.M{"album_id": albumID}
	if mediaType != "" {
		filter["type"] = mediaType
	}

	// Get total count
	total, err := r.albumMediaCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	// Get paginated data
	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "created_at", Value: -1}})
	findOptions.SetLimit(limit)
	findOptions.SetSkip(offset)

	cursor, err := r.albumMediaCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var media []models.AlbumMedia
	if err := cursor.All(ctx, &media); err != nil {
		return nil, 0, err
	}
	return media, total, nil
}

func (r *FeedRepository) GetTimelineMedia(ctx context.Context, userID primitive.ObjectID, limit, offset int64, mediaType string) ([]models.AlbumMedia, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	matchStage := bson.D{
		{Key: "$match", Value: bson.D{
			{Key: "user_id", Value: userID},
			{Key: "media", Value: bson.D{{Key: "$exists", Value: true}, {Key: "$not", Value: bson.D{{Key: "$size", Value: 0}}}}},
		}},
	}

	unwindStage := bson.D{{Key: "$unwind", Value: "$media"}}

	pipeline := mongo.Pipeline{matchStage, unwindStage}

	// Filter by media type after unwind
	if mediaType != "" {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{{Key: "media.type", Value: mediaType}}}})
	}

	// Count total - run aggregation with count
	countPipeline := append(pipeline, bson.D{{Key: "$count", Value: "total"}})
	countCursor, err := r.postsCollection.Aggregate(ctx, countPipeline)
	if err != nil {
		return nil, 0, err
	}
	var countResult []bson.M
	if err := countCursor.All(ctx, &countResult); err != nil {
		return nil, 0, err
	}
	var total int64
	if len(countResult) > 0 {
		if t, ok := countResult[0]["total"].(int32); ok {
			total = int64(t)
		}
	}

	// Get paginated data
	pipeline = append(pipeline,
		bson.D{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}}}},
		bson.D{{Key: "$skip", Value: offset}},
		bson.D{{Key: "$limit", Value: limit}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "user_id", Value: 1},
			{Key: "url", Value: "$media.url"},
			{Key: "type", Value: "$media.type"},
			{Key: "caption", Value: "$content"},
			{Key: "created_at", Value: 1},
		}}},
	)

	cursor, err := r.postsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var media []models.AlbumMedia
	if err := cursor.All(ctx, &media); err != nil {
		return nil, 0, err
	}

	return media, total, nil
}

func (r *FeedRepository) UpdateAlbumCover(ctx context.Context, albumID primitive.ObjectID, coverURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.albumsCollection.UpdateOne(
		ctx,
		bson.M{"_id": albumID},
		bson.M{
			"$set": bson.M{"cover_url": coverURL, "updated_at": time.Now()},
		},
	)
	return err
}

func (r *FeedRepository) RemoveAlbumCoverByURL(ctx context.Context, coverURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.albumsCollection.UpdateMany(
		ctx,
		bson.M{"cover_url": coverURL},
		bson.M{
			"$set": bson.M{"cover_url": "", "updated_at": time.Now()},
		},
	)
	return err
}

// UpdateAlbum updates an album's properties (name, description, cover, privacy)
func (r *FeedRepository) UpdateAlbum(ctx context.Context, albumID, userID primitive.ObjectID, update bson.M) (*models.Album, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	update["updated_at"] = time.Now()

	// Only allow update if user owns the album
	result := r.albumsCollection.FindOneAndUpdate(
		ctx,
		bson.M{"_id": albumID, "user_id": userID},
		bson.M{"$set": update},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)

	var album models.Album
	if err := result.Decode(&album); err != nil {
		return nil, err
	}
	return &album, nil
}

// ----------------------------- Posts -----------------------------

func (r *FeedRepository) CreatePost(ctx context.Context, post *models.Post) (*models.Post, error) {
	post.CreatedAt = time.Now()
	post.UpdatedAt = time.Now()
	res, err := r.postsCollection.InsertOne(ctx, post)
	if err != nil {
		return nil, err
	}
	post.ID = res.InsertedID.(primitive.ObjectID)
	return post, nil
}

func (r *FeedRepository) GetPostByID(ctx context.Context, postID primitive.ObjectID) (*models.Post, error) {
	var post models.Post
	err := r.postsCollection.FindOne(ctx, bson.M{"_id": postID}).Decode(&post)
	if err != nil {
		return nil, err
	}
	return &post, nil
}

func (r *FeedRepository) UpdatePost(ctx context.Context, postID primitive.ObjectID, update bson.M) (*models.Post, error) {
	update["updated_at"] = time.Now()
	res := r.postsCollection.FindOneAndUpdate(
		ctx,
		bson.M{"_id": postID},
		bson.M{"$set": update},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var updatedPost models.Post
	if err := res.Decode(&updatedPost); err != nil {
		return nil, err
	}
	return &updatedPost, nil
}

func (r *FeedRepository) DeletePost(ctx context.Context, userID, postID primitive.ObjectID) error {
	res, err := r.postsCollection.DeleteOne(ctx, bson.M{"_id": postID, "user_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (r *FeedRepository) DeleteCommentsByPostID(ctx context.Context, postID primitive.ObjectID) error {
	_, err := r.commentsCollection.DeleteMany(ctx, bson.M{"post_id": postID})
	return err
}

func (r *FeedRepository) DeleteReactionsByTargetID(ctx context.Context, targetID primitive.ObjectID) error {
	_, err := r.reactionsCollection.DeleteMany(ctx, bson.M{"target_id": targetID})
	return err
}

func (r *FeedRepository) GetCommentIDsByPostID(ctx context.Context, postID primitive.ObjectID) ([]primitive.ObjectID, error) {
	// Projection to only return _id
	opts := options.Find().SetProjection(bson.M{"_id": 1})
	cur, err := r.commentsCollection.Find(ctx, bson.M{"post_id": postID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var results []struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	if err := cur.All(ctx, &results); err != nil {
		return nil, err
	}

	ids := make([]primitive.ObjectID, len(results))
	for i, res := range results {
		ids[i] = res.ID
	}
	return ids, nil
}

func (r *FeedRepository) GetReplyIDsByCommentIDs(ctx context.Context, commentIDs []primitive.ObjectID) ([]primitive.ObjectID, error) {
	if len(commentIDs) == 0 {
		return []primitive.ObjectID{}, nil
	}
	opts := options.Find().SetProjection(bson.M{"_id": 1})
	cur, err := r.repliesCollection.Find(ctx, bson.M{"comment_id": bson.M{"$in": commentIDs}}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var results []struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	if err := cur.All(ctx, &results); err != nil {
		return nil, err
	}

	ids := make([]primitive.ObjectID, len(results))
	for i, res := range results {
		ids[i] = res.ID
	}
	return ids, nil
}

func (r *FeedRepository) DeleteRepliesByCommentIDs(ctx context.Context, commentIDs []primitive.ObjectID) error {
	if len(commentIDs) == 0 {
		return nil
	}
	_, err := r.repliesCollection.DeleteMany(ctx, bson.M{"comment_id": bson.M{"$in": commentIDs}})
	return err
}

func (r *FeedRepository) DeleteReactionsByTargetIDs(ctx context.Context, targetIDs []primitive.ObjectID) error {
	if len(targetIDs) == 0 {
		return nil
	}
	_, err := r.reactionsCollection.DeleteMany(ctx, bson.M{"target_id": bson.M{"$in": targetIDs}})
	return err
}

func (r *FeedRepository) DeleteAlbumMediaByURL(ctx context.Context, url string) error {
	_, err := r.albumMediaCollection.DeleteMany(ctx, bson.M{"url": url})
	return err
}

func (r *FeedRepository) ListPosts(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]models.Post, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
	}
	pipeline = append(pipeline, r.aggregatePostPipeline()...)

	// sort/skip/limit from opts
	if opts != nil {
		if opts.Sort != nil {
			pipeline = append(pipeline, bson.D{{Key: "$sort", Value: opts.Sort}})
		}
		if opts.Skip != nil {
			pipeline = append(pipeline, bson.D{{Key: "$skip", Value: *opts.Skip}})
		}
		if opts.Limit != nil {
			pipeline = append(pipeline, bson.D{{Key: "$limit", Value: *opts.Limit}})
		}
	}

	cur, err := r.postsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var posts []models.Post
	if err := cur.All(ctx, &posts); err != nil {
		return nil, err
	}
	if posts == nil {
		posts = []models.Post{}
	}
	return posts, nil
}

func (r *FeedRepository) CountPosts(ctx context.Context, filter bson.M) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	return r.postsCollection.CountDocuments(ctx, filter)
}

// --------------------------- Comments ----------------------------

func (r *FeedRepository) CreateComment(ctx context.Context, comment *models.Comment) (*models.Comment, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	now := time.Now()
	comment.CreatedAt = now
	comment.UpdatedAt = now

	res, err := r.commentsCollection.InsertOne(ctx, comment)
	if err != nil {
		return nil, err
	}
	comment.ID = res.InsertedID.(primitive.ObjectID)

	// push comment id to post
	_, err = r.postsCollection.UpdateOne(
		ctx,
		bson.M{"_id": comment.PostID},
		bson.M{"$push": bson.M{"comment_ids": comment.ID}},
	)
	if err != nil {
		return nil, err
	}

	return comment, nil
}

func (r *FeedRepository) GetCommentByID(ctx context.Context, commentID primitive.ObjectID) (*models.Comment, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"_id": commentID}}},
	}
	pipeline = append(pipeline, r.aggregateCommentPipeline()...)

	cur, err := r.commentsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	if cur.Next(ctx) {
		var c models.Comment
		if err := cur.Decode(&c); err != nil {
			return nil, err
		}
		return &c, nil
	}
	return nil, mongo.ErrNoDocuments
}

func (r *FeedRepository) UpdateComment(ctx context.Context, commentID primitive.ObjectID, update bson.M) (*models.Comment, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	update["updated_at"] = time.Now()
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	res := r.commentsCollection.FindOneAndUpdate(ctx, bson.M{"_id": commentID}, bson.M{"$set": update}, opts)
	var updated models.Comment
	if err := res.Decode(&updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

func (r *FeedRepository) DeleteComment(ctx context.Context, postID, commentID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// pull id from post
	if _, err := r.postsCollection.UpdateOne(ctx, bson.M{"_id": postID}, bson.M{"$pull": bson.M{"comment_ids": commentID}}); err != nil {
		return err
	}
	_, err := r.commentsCollection.DeleteOne(ctx, bson.M{"_id": commentID})
	return err
}

// ---------------------------- Replies ----------------------------

func (r *FeedRepository) CreateReply(ctx context.Context, reply *models.Reply) (*models.Reply, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	now := time.Now()
	reply.CreatedAt = now
	reply.UpdatedAt = now

	res, err := r.repliesCollection.InsertOne(ctx, reply)
	if err != nil {
		return nil, err
	}
	reply.ID = res.InsertedID.(primitive.ObjectID)

	_, err = r.commentsCollection.UpdateOne(
		ctx,
		bson.M{"_id": reply.CommentID},
		bson.M{"$push": bson.M{"replyids": reply.ID}},
	)
	if err != nil {
		return nil, err
	}

	return reply, nil
}

func (r *FeedRepository) GetReplyByID(ctx context.Context, replyID primitive.ObjectID) (*models.Reply, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"_id": replyID}}},
	}
	pipeline = append(pipeline, r.aggregateReplyPipeline()...)

	cur, err := r.repliesCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	if cur.Next(ctx) {
		var rp models.Reply
		if err := cur.Decode(&rp); err != nil {
			return nil, err
		}
		return &rp, nil
	}
	return nil, mongo.ErrNoDocuments
}

func (r *FeedRepository) UpdateReply(ctx context.Context, replyID primitive.ObjectID, update bson.M) (*models.Reply, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	update["updated_at"] = time.Now()
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	res := r.repliesCollection.FindOneAndUpdate(ctx, bson.M{"_id": replyID}, bson.M{"$set": update}, opts)
	var updated models.Reply
	if err := res.Decode(&updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

func (r *FeedRepository) DeleteReply(ctx context.Context, commentID, replyID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if _, err := r.commentsCollection.UpdateOne(ctx, bson.M{"_id": commentID}, bson.M{"$pull": bson.M{"replyids": replyID}}); err != nil {
		return err
	}
	_, err := r.repliesCollection.DeleteOne(ctx, bson.M{"_id": replyID})
	return err
}

// --------------------------- Reactions ---------------------------

func (r *FeedRepository) ListReactions(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]models.Reaction, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
	}

	if opts != nil {
		if opts.Sort != nil {
			pipeline = append(pipeline, bson.D{{Key: "$sort", Value: opts.Sort}})
		}
		if opts.Skip != nil {
			pipeline = append(pipeline, bson.D{{Key: "$skip", Value: *opts.Skip}})
		}
		if opts.Limit != nil {
			pipeline = append(pipeline, bson.D{{Key: "$limit", Value: *opts.Limit}})
		}
	}

	cursor, err := r.reactionsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var reactions []models.Reaction
	if err := cursor.All(ctx, &reactions); err != nil {
		return nil, err
	}

	return reactions, nil
}

func (r *FeedRepository) CreateReaction(ctx context.Context, reaction *models.Reaction) (*models.Reaction, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	reaction.CreatedAt = time.Now()

	res, err := r.reactionsCollection.InsertOne(ctx, reaction)
	if err != nil {
		return nil, err
	}
	reaction.ID = res.InsertedID.(primitive.ObjectID)
	return reaction, nil
}

func (r *FeedRepository) GetReactionByID(ctx context.Context, reactionID primitive.ObjectID) (*models.Reaction, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var reaction models.Reaction
	if err := r.reactionsCollection.FindOne(ctx, bson.M{"_id": reactionID}).Decode(&reaction); err != nil {
		return nil, err
	}
	return &reaction, nil
}

func (r *FeedRepository) DeleteReaction(ctx context.Context, reactionID, userID, targetID primitive.ObjectID, targetType string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.reactionsCollection.DeleteOne(
		ctx,
		bson.M{"_id": reactionID, "user_id": userID, "target_id": targetID, "target_type": targetType},
	)
	return err
}

// ----------------------------- Lists -----------------------------

func (r *FeedRepository) ListComments(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]models.Comment, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
	}
	pipeline = append(pipeline, r.aggregateCommentPipeline()...)

	if opts != nil {
		if opts.Sort != nil {
			pipeline = append(pipeline, bson.D{{Key: "$sort", Value: opts.Sort}})
		}
		if opts.Skip != nil {
			pipeline = append(pipeline, bson.D{{Key: "$skip", Value: *opts.Skip}})
		}
		if opts.Limit != nil {
			pipeline = append(pipeline, bson.D{{Key: "$limit", Value: *opts.Limit}})
		}
	}

	cur, err := r.commentsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var comments []models.Comment
	if err := cur.All(ctx, &comments); err != nil {
		return nil, err
	}
	return comments, nil
}

func (r *FeedRepository) ListReplies(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]models.Reply, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
	}
	pipeline = append(pipeline, r.aggregateReplyPipeline()...)

	if opts != nil {
		if opts.Sort != nil {
			pipeline = append(pipeline, bson.D{{Key: "$sort", Value: opts.Sort}})
		}
		if opts.Skip != nil {
			pipeline = append(pipeline, bson.D{{Key: "$skip", Value: *opts.Skip}})
		}
		if opts.Limit != nil {
			pipeline = append(pipeline, bson.D{{Key: "$limit", Value: *opts.Limit}})
		}
	}

	cur, err := r.repliesCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var replies []models.Reply
	if err := cur.All(ctx, &replies); err != nil {
		return nil, err
	}
	return replies, nil
}

// ------------------------ Aggregation helpers --------------------

func (r *FeedRepository) aggregatePostPipeline() mongo.Pipeline {
	return mongo.Pipeline{
		// Lookup user for the post (author)
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "user_id",
			"foreignField": "_id",
			"as":           "author_info",
		}}},
		bson.D{{Key: "$unwind", Value: bson.M{"path": "$author_info", "preserveNullAndEmptyArrays": true}}},

		// Lookup reactions and count by type
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "reactions",
			"localField":   "_id",
			"foreignField": "target_id",
			"as":           "reaction_counts_array", // Temporary name for the array of {k,v} pairs
			"pipeline": mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.M{"target_type": "post"}}},
				bson.D{{Key: "$group", Value: bson.M{
					"_id":   "$type",
					"count": bson.M{"$sum": 1},
				}}},
				bson.D{{Key: "$project", Value: bson.M{
					"_id": 0,        // Exclude _id from the sub-document
					"k":   "$_id",   // Key for $arrayToObject
					"v":   "$count", // Value for $arrayToObject
				}}},
			},
		}}},
		// Convert array of {k, v} objects into a single object
		bson.D{{Key: "$addFields", Value: bson.M{
			"specific_reaction_counts": bson.M{"$arrayToObject": "$reaction_counts_array"},
		}}},
		// Lookup mentioned users
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "mentions",
			"foreignField": "_id",
			"as":           "mentioned_users_info",
		}}},
		// Final projection (shape the output as needed)
		bson.D{{Key: "$project", Value: bson.M{
			"_id":             1,
			"id":              bson.M{"$toString": "$_id"},
			"user_id":         1,
			"content":         1,
			"media_type":      1,
			"media_url":       1,
			"media":           1,
			"privacy":         1,
			"custom_audience": 1,
			"mentions":        1,
			"mentioned_users": bson.M{
				"$map": bson.M{
					"input": "$mentioned_users_info",
					"as":    "u",
					"in": bson.M{
						"id":        bson.M{"$toString": "$$u._id"},
						"username":  "$$u.username",
						"avatar":    "$$u.avatar",
						"full_name": "$$u.full_name",
					},
				},
			},
			"location":   1,
			"hashtags":   1,
			"created_at": 1,
			"updated_at": 1,
			"author": bson.M{
				"id":        bson.M{"$toString": "$author_info._id"},
				"username":  bson.M{"$ifNull": bson.A{"$author_info.username", "Deleted User"}},
				"avatar":    bson.M{"$ifNull": bson.A{"$author_info.avatar", ""}},
				"full_name": bson.M{"$ifNull": bson.A{"$author_info.full_name", "Deleted User"}},
			},
			"specific_reaction_counts": "$specific_reaction_counts",
			"total_reactions":          "$total_reactions",
			"total_comments":           "$total_comments",
		}}},
	}
}

func (r *FeedRepository) aggregateCommentPipeline() mongo.Pipeline {
	return mongo.Pipeline{
		// comment author
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "user_id",
			"foreignField": "_id",
			"as":           "author_info",
		}}},
		bson.D{{Key: "$unwind", Value: bson.M{"path": "$author_info", "preserveNullAndEmptyArrays": true}}},

		// replies for comment
		bson.D{{Key: "$lookup", Value: bson.M{
			"from": "replies",
			"let":  bson.M{"commentId": "$_id"},
			"pipeline": mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.M{"$expr": bson.M{"$eq": bson.A{"$comment_id", "$$commentId"}}}}},
				bson.D{{Key: "$lookup", Value: bson.M{
					"from":         "users",
					"localField":   "user_id",
					"foreignField": "_id",
					"as":           "author_info",
				}}},
				bson.D{{Key: "$unwind", Value: bson.M{"path": "$author_info", "preserveNullAndEmptyArrays": true}}},
				bson.D{{Key: "$project", Value: bson.M{
					"_id":             1,
					"comment_id":      1,
					"parent_reply_id": 1,
					"user_id":         1,
					"content":         1,
					"media_type":      1,
					"media_url":       1,
					"mentions":        1,
					"created_at":      1,
					"updated_at":      1,
					"author": bson.M{
						"id":        "$author_info._id",
						"username":  "$author_info.username",
						"avatar":    "$author_info.avatar",
						"full_name": "$author_info.full_name",
					},
				}}},
			},
			"as": "replies",
		}}},

		// final shape
		bson.D{{Key: "$project", Value: bson.M{
			"_id":        1,
			"id":         bson.M{"$toString": "$_id"},
			"post_id":    1,
			"user_id":    1,
			"content":    1,
			"media_type": 1,
			"media_url":  1,
			"mentions":   1,
			"created_at": 1,
			"updated_at": 1,
			"replies":    1,
			"author": bson.M{
				"id":        bson.M{"$toString": "$author_info._id"},
				"username":  "$author_info.username",
				"avatar":    "$author_info.avatar",
				"full_name": "$author_info.full_name",
			},
		}}},
	}
}

func (r *FeedRepository) aggregateReplyPipeline() mongo.Pipeline {
	return mongo.Pipeline{
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "user_id",
			"foreignField": "_id",
			"as":           "author_info",
		}}},
		bson.D{{Key: "$unwind", Value: bson.M{"path": "$author_info", "preserveNullAndEmptyArrays": true}}},
		bson.D{{Key: "$project", Value: bson.M{
			"_id":             1,
			"id":              bson.M{"$toString": "$_id"},
			"comment_id":      1,
			"parent_reply_id": 1,
			"user_id":         1,
			"content":         1,
			"media_type":      1,
			"media_url":       1,
			"mentions":        1,
			"created_at":      1,
			"updated_at":      1,
			"author": bson.M{
				"id":        bson.M{"$toString": "$author_info._id"},
				"username":  "$author_info.username",
				"avatar":    "$author_info.avatar",
				"full_name": "$author_info.full_name",
			},
		}}},
	}
}

// ----------------------- Reaction analytics ----------------------

func (r *FeedRepository) CountReactionsByType(ctx context.Context, targetID primitive.ObjectID, targetType string) (map[string]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"target_id": targetID, "target_type": targetType}}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$type", "count": bson.M{"$sum": 1}}}},
	}

	cur, err := r.reactionsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate reaction counts: %w", err)
	}
	defer cur.Close(ctx)

	out := make(map[string]int64)
	for cur.Next(ctx) {
		var row struct {
			Type  string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, fmt.Errorf("failed to decode reaction count result: %w", err)
		}
		out[row.Type] = row.Count
	}

	return out, nil
}

func (r *FeedRepository) IncrementPostReactionCount(ctx context.Context, postID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.postsCollection.UpdateOne(
		ctx,
		bson.M{"_id": postID},
		bson.M{"$inc": bson.M{"total_reactions": 1}},
	)
	return err
}

func (r *FeedRepository) DecrementPostReactionCount(ctx context.Context, postID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.postsCollection.UpdateOne(
		ctx,
		bson.M{"_id": postID},
		bson.M{"$inc": bson.M{"total_reactions": -1}},
	)
	return err
}

func (r *FeedRepository) IncrementCommentReactionCount(ctx context.Context, commentID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.commentsCollection.UpdateOne(
		ctx,
		bson.M{"_id": commentID},
		bson.M{"$inc": bson.M{"total_reactions": 1}},
	)
	return err
}

func (r *FeedRepository) DecrementCommentReactionCount(ctx context.Context, commentID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.commentsCollection.UpdateOne(
		ctx,
		bson.M{"_id": commentID},
		bson.M{"$inc": bson.M{"total_reactions": -1}},
	)
	return err
}

func (r *FeedRepository) IncrementReplyReactionCount(ctx context.Context, replyID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.repliesCollection.UpdateOne(
		ctx,
		bson.M{"_id": replyID},
		bson.M{"$inc": bson.M{"total_reactions": 1}},
	)
	return err
}

func (r *FeedRepository) DecrementReplyReactionCount(ctx context.Context, replyID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.repliesCollection.UpdateOne(
		ctx,
		bson.M{"_id": replyID},
		bson.M{"$inc": bson.M{"total_reactions": -1}},
	)
	return err
}

func (r *FeedRepository) IncrementPostCommentCount(ctx context.Context, postID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.postsCollection.UpdateOne(
		ctx,
		bson.M{"_id": postID},
		bson.M{"$inc": bson.M{"total_comments": 1}},
	)
	return err
}

func (r *FeedRepository) DecrementPostCommentCount(ctx context.Context, postID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	_, err := r.postsCollection.UpdateOne(
		ctx,
		bson.M{"_id": postID},
		bson.M{"$inc": bson.M{"total_comments": -1}},
	)
	return err
}
