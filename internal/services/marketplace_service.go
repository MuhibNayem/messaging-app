package services

import (
	"context"
	"errors"
	"gitlab.com/spydotech-group/shared-entity/models"
	"messaging-app/internal/repositories"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type MarketplaceService struct {
	repo                 *repositories.MarketplaceRepository
	userRepo             *repositories.UserRepository
	messageCassandraRepo *repositories.MessageCassandraRepository
}

func NewMarketplaceService(repo *repositories.MarketplaceRepository, userRepo *repositories.UserRepository, mcr *repositories.MessageCassandraRepository) *MarketplaceService {
	return &MarketplaceService{
		repo:                 repo,
		userRepo:             userRepo,
		messageCassandraRepo: mcr,
	}
}

func (s *MarketplaceService) GetCategories(ctx context.Context) ([]models.Category, error) {
	// TODO: Add caching here (Redis)
	return s.repo.GetCategories(ctx)
}

func (s *MarketplaceService) CreateProduct(ctx context.Context, userID primitive.ObjectID, req models.CreateProductRequest) (*models.Product, error) {
	catID, err := primitive.ObjectIDFromHex(req.CategoryID)
	if err != nil {
		return nil, errors.New("invalid category ID")
	}

	product := &models.Product{
		SellerID:    userID,
		CategoryID:  catID,
		Title:       req.Title,
		Description: req.Description,
		Price:       req.Price,
		Currency:    req.Currency,
		Images:      req.Images,
		Location:    req.Location,
		Status:      models.ProductStatusAvailable,
		Tags:        req.Tags,
	}

	return s.repo.CreateProduct(ctx, product)
}

func (s *MarketplaceService) GetProductByID(ctx context.Context, id primitive.ObjectID, viewerID primitive.ObjectID) (*models.ProductResponse, error) {
	// Increment Views (Fire and Forget)
	go func() {
		// Create a detached context with timeout to ensure view increment runs even if request returns
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.repo.IncrementViews(bgCtx, id)
	}()

	product, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Fetch Seller Info
	seller, err := s.userRepo.FindUserByID(ctx, product.SellerID)
	if err != nil {
		return nil, errors.New("failed to fetch seller info")
	}

	// Fetch Category Info (Ideally cached)
	categories, err := s.repo.GetCategories(ctx)
	var category models.Category
	if err == nil {
		for _, cat := range categories {
			if cat.ID == product.CategoryID {
				category = cat
				break
			}
		}
	}

	// Check if Viewer saved this product
	isSaved := false
	for _, savedID := range product.SavedBy {
		if savedID == viewerID {
			isSaved = true
			break
		}
	}

	return &models.ProductResponse{
		ID:          product.ID,
		Title:       product.Title,
		Description: product.Description,
		Price:       product.Price,
		Currency:    product.Currency,
		Images:      product.Images,
		Location:    product.Location,
		Status:      product.Status,
		Tags:        product.Tags,
		Views:       product.Views, // View count might be slightly stale, acceptable
		CreatedAt:   product.CreatedAt,
		Seller: models.UserShortResponse{
			ID:       seller.ID,
			Username: seller.Username,
			FullName: seller.FullName,
			Avatar:   seller.Avatar,
		},
		Category: category,
		IsSaved:  isSaved,
	}, nil
}

// Quick fix: Let's create a MarketplaceListResponse in this file or models
type MarketplaceListResponse struct {
	Products []models.ProductResponse `json:"products"`
	Total    int64                    `json:"total"`
	Page     int64                    `json:"page"`
	Limit    int64                    `json:"limit"`
}

func (s *MarketplaceService) SearchProducts(ctx context.Context, filter models.ProductFilter) (*MarketplaceListResponse, error) {
	products, total, err := s.repo.ListProducts(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &MarketplaceListResponse{
		Products: products,
		Total:    total,
		Page:     filter.Page,
		Limit:    filter.Limit,
	}, nil
}

func (s *MarketplaceService) GetMarketplaceConversations(ctx context.Context, userID primitive.ObjectID) ([]models.ConversationSummary, error) {
	// Use Cassandra for scalable marketplace inbox
	return s.messageCassandraRepo.GetInbox(ctx, userID, true) // isMarketplace = true
}

func (s *MarketplaceService) MarkProductSold(ctx context.Context, productID, userID primitive.ObjectID) error {
	product, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return err
	}

	if product.SellerID != userID {
		return errors.New("unauthorized")
	}

	_, err = s.repo.UpdateProduct(ctx, productID, bson.M{"status": models.ProductStatusSold})
	return err
}

func (s *MarketplaceService) DeleteProduct(ctx context.Context, productID, userID primitive.ObjectID) error {
	product, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return err
	}
	if product.SellerID != userID {
		return errors.New("unauthorized")
	}
	return s.repo.DeleteProduct(ctx, productID)
}

func (s *MarketplaceService) ToggleSaveProduct(ctx context.Context, productID, userID primitive.ObjectID) (bool, error) {
	product, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return false, err
	}

	isSaved := false
	for _, id := range product.SavedBy {
		if id == userID {
			isSaved = true
			break
		}
	}

	var update bson.M
	if isSaved {
		update = bson.M{"$pull": bson.M{"saved_by": userID}}
	} else {
		update = bson.M{"$addToSet": bson.M{"saved_by": userID}}
	}

	_, err = s.repo.UpdateProduct(ctx, productID, update)
	if err != nil {
		return false, err
	}

	return !isSaved, nil
}
