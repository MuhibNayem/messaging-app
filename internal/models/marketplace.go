package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ProductStatus string

const (
	ProductStatusAvailable ProductStatus = "available"
	ProductStatusSold      ProductStatus = "sold"
	ProductStatusPending   ProductStatus = "pending"
	ProductStatusArchived  ProductStatus = "archived"
)

type Product struct {
	ID          primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	SellerID    primitive.ObjectID   `bson:"seller_id" json:"seller_id"`
	CategoryID  primitive.ObjectID   `bson:"category_id" json:"category_id"`
	Title       string               `bson:"title" json:"title"`
	Description string               `bson:"description" json:"description"`
	Price       float64              `bson:"price" json:"price"`
	Currency    string               `bson:"currency" json:"currency"` // e.g., "BDT", "USD"
	Images      []string             `bson:"images" json:"images"`
	Location    string               `bson:"location" json:"location"`                           // Text representation for now
	Coordinates []float64            `bson:"coordinates,omitempty" json:"coordinates,omitempty"` // [Longitude, Latitude] for GeoJSON
	Status      ProductStatus        `bson:"status" json:"status"`
	SavedBy     []primitive.ObjectID `bson:"saved_by,omitempty" json:"saved_by,omitempty"`
	Tags        []string             `bson:"tags,omitempty" json:"tags,omitempty"`
	Views       int64                `bson:"views" json:"views"`
	CreatedAt   time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time            `bson:"updated_at" json:"updated_at"`
}

type Category struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name      string             `bson:"name" json:"name"`
	Slug      string             `bson:"slug" json:"slug"` // Unique identifier (e.g., "electronics")
	Icon      string             `bson:"icon" json:"icon"` // Name of Lucide icon or URL
	Order     int                `bson:"order" json:"order"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}

// ProductResponse is for API responses, potentially including expanded Seller/Category info
type ProductResponse struct {
	ID          primitive.ObjectID `bson:"_id" json:"id"`
	Title       string             `bson:"title" json:"title"`
	Description string             `bson:"description" json:"description"`
	Price       float64            `bson:"price" json:"price"`
	Currency    string             `bson:"currency" json:"currency"`
	Images      []string           `bson:"images" json:"images"`
	Location    string             `bson:"location" json:"location"`
	Status      ProductStatus      `bson:"status" json:"status"`
	Tags        []string           `bson:"tags,omitempty" json:"tags,omitempty"`
	Views       int64              `bson:"views" json:"views"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	Seller      UserShortResponse  `bson:"seller" json:"seller"`
	Category    Category           `bson:"category" json:"category"`
	IsSaved     bool               `bson:"is_saved" json:"is_saved"` // If the requesting user has saved this
}

type CreateProductRequest struct {
	Title       string   `json:"title" binding:"required"`
	Description string   `json:"description" binding:"required"`
	Price       float64  `json:"price" binding:"required"`
	Currency    string   `json:"currency" binding:"required"`
	CategoryID  string   `json:"category_id" binding:"required"`
	Images      []string `json:"images" binding:"required,min=1"` // At least one image required
	Location    string   `json:"location" binding:"required"`
	Tags        []string `json:"tags,omitempty"`
}

type UpdateProductRequest struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	Price       *float64       `json:"price,omitempty"`
	Currency    string         `json:"currency,omitempty"`
	CategoryID  string         `json:"category_id,omitempty"`
	Images      []string       `json:"images,omitempty"`
	Location    string         `json:"location,omitempty"`
	Status      *ProductStatus `json:"status,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
}

type ProductFilter struct {
	Query      string   `form:"q"`
	CategoryID string   `form:"category_id"`
	MinPrice   *float64 `form:"min_price"`
	MaxPrice   *float64 `form:"max_price"`
	Location   string   `form:"location"` // Basic filtering
	SortBy     string   `form:"sort_by"`  // "price_asc", "price_desc", "newest"
	Page       int64    `form:"page,default=1"`
	Limit      int64    `form:"limit,default=20"`
}
