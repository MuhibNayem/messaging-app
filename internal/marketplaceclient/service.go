package marketplaceclient

import (
	"context"
	"time"

	"gitlab.com/spydotech-group/shared-entity/models"
	marketplacepb "gitlab.com/spydotech-group/shared-entity/proto/marketplace/v1"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"google.golang.org/protobuf/types/known/emptypb"
)

// CreateProduct creates a new product listing
func (c *Client) CreateProduct(ctx context.Context, userID primitive.ObjectID, req models.CreateProductRequest) (*models.Product, error) {
	// Convert location string to proto Location
	protoReq := &marketplacepb.CreateProductRequest{
		UserId:      userID.Hex(),
		CategoryId:  req.CategoryID,
		Title:       req.Title,
		Description: req.Description,
		Price:       req.Price,
		Currency:    req.Currency,
		Images:      req.Images,
		Location: &marketplacepb.Location{
			City: req.Location, // Models use string, proto uses struct
		},
		Tags: req.Tags,
	}

	resp, err := c.client.CreateProduct(ctx, protoReq)
	if err != nil {
		return nil, err
	}

	return protoProductToModel(resp.Product), nil
}

// GetProduct retrieves a product by ID
func (c *Client) GetProduct(ctx context.Context, productID, viewerID primitive.ObjectID) (*models.ProductResponse, error) {
	req := &marketplacepb.GetProductRequest{
		ProductId: productID.Hex(),
		ViewerId:  viewerID.Hex(),
	}

	resp, err := c.client.GetProduct(ctx, req)
	if err != nil {
		return nil, err
	}

	return protoProductToResponse(resp.Product), nil
}

// SearchProducts searches for products with filters
func (c *Client) SearchProducts(ctx context.Context, filter models.ProductFilter) ([]models.ProductResponse, int64, error) {
	req := &marketplacepb.SearchProductsRequest{
		CategoryId: filter.CategoryID,
		Query:      filter.Query,
		MinPrice:   *filter.MinPrice,
		MaxPrice:   *filter.MaxPrice,
		SortBy:     filter.SortBy,
		Page:       filter.Page,
		Limit:      filter.Limit,
	}

	resp, err := c.client.SearchProducts(ctx, req)
	if err != nil {
		return nil, 0, err
	}

	products := make([]models.ProductResponse, len(resp.Products))
	for i, p := range resp.Products {
		products[i] = *protoProductToResponse(p)
	}

	return products, resp.Total, nil
}

// GetCategories retrieves all product categories
func (c *Client) GetCategories(ctx context.Context) ([]models.Category, error) {
	resp, err := c.client.GetCategories(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}

	categories := make([]models.Category, len(resp.Categories))
	for i, cat := range resp.Categories {
		id, _ := primitive.ObjectIDFromHex(cat.Id)
		categories[i] = models.Category{
			ID:    id,
			Name:  cat.Name,
			Slug:  cat.Slug,
			Icon:  cat.Icon,
			Order: int(cat.Order),
		}
	}

	return categories, nil
}

// ToggleSaveProduct saves or unsaves a product
func (c *Client) ToggleSaveProduct(ctx context.Context, productID, userID primitive.ObjectID) (bool, error) {
	req := &marketplacepb.ToggleSaveProductRequest{
		ProductId: productID.Hex(),
		UserId:    userID.Hex(),
	}

	resp, err := c.client.ToggleSaveProduct(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.IsSaved, nil
}

// MarkProductSold marks a product as sold
func (c *Client) MarkProductSold(ctx context.Context, productID, userID primitive.ObjectID) error {
	req := &marketplacepb.MarkProductSoldRequest{
		ProductId: productID.Hex(),
		UserId:    userID.Hex(),
	}

	_, err := c.client.MarkProductSold(ctx, req)
	return err
}

// DeleteProduct deletes a product
func (c *Client) DeleteProduct(ctx context.Context, productID, userID primitive.ObjectID) error {
	req := &marketplacepb.DeleteProductRequest{
		ProductId: productID.Hex(),
		UserId:    userID.Hex(),
	}

	_, err := c.client.DeleteProduct(ctx, req)
	return err
}

// GetMarketplaceConversations retrieves marketplace conversations for a user
func (c *Client) GetMarketplaceConversations(ctx context.Context, userID primitive.ObjectID) ([]models.ConversationSummary, error) {
	req := &marketplacepb.GetConversationsRequest{
		UserId: userID.Hex(),
	}

	resp, err := c.client.GetMarketplaceConversations(ctx, req)
	if err != nil {
		return nil, err
	}

	conversations := make([]models.ConversationSummary, len(resp.Conversations))
	for i, conv := range resp.Conversations {
		senderID, _ := primitive.ObjectIDFromHex(conv.LastMessageSenderId)
		var timestamp *time.Time
		if conv.LastMessageTimestamp != nil {
			t := conv.LastMessageTimestamp.AsTime()
			timestamp = &t
		}

		conversations[i] = models.ConversationSummary{
			ID:                     conv.Id,
			Name:                   conv.Name,
			Avatar:                 conv.Avatar,
			IsGroup:                conv.IsGroup,
			LastMessageContent:     conv.LastMessageContent,
			LastMessageTimestamp:   timestamp,
			LastMessageSenderID:    senderID,
			UnreadCount:            int64(conv.UnreadCount),
			LastMessageIsEncrypted: conv.LastMessageIsEncrypted,
		}
	}

	return conversations, nil
}
