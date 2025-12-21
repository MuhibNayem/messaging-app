package seeds

import (
	"context"
	"log"
	"gitlab.com/spydotech-group/shared-entity/models"
	"messaging-app/internal/repositories"
	"time"
)

type MarketplaceSeeder struct {
	repo *repositories.MarketplaceRepository
}

func NewMarketplaceSeeder(repo *repositories.MarketplaceRepository) *MarketplaceSeeder {
	return &MarketplaceSeeder{repo: repo}
}

func (s *MarketplaceSeeder) SeedCategories(ctx context.Context) error {
	log.Println("Seeding marketplace categories...")

	categories := []models.Category{
		{Name: "Vehicles", Slug: "vehicles", Icon: "Car", Order: 1},
		{Name: "Property Rentals", Slug: "property-rentals", Icon: "Home", Order: 2},
		{Name: "Apparel", Slug: "apparel", Icon: "Shirt", Order: 3},
		{Name: "Electronics", Slug: "electronics", Icon: "Smartphone", Order: 4},
		{Name: "Entertainment", Slug: "entertainment", Icon: "Film", Order: 5},
		{Name: "Family", Slug: "family", Icon: "Baby", Order: 6},
		{Name: "Free Stuff", Slug: "free-stuff", Icon: "Gift", Order: 7},
		{Name: "Garden & Outdoor", Slug: "garden", Icon: "Flower", Order: 8},
		{Name: "Home Goods", Slug: "home-goods", Icon: "Sofa", Order: 9},
		{Name: "Office Supplies", Slug: "office", Icon: "Paperclip", Order: 10},
		{Name: "Pet Supplies", Slug: "pet-supplies", Icon: "Dog", Order: 11},
		{Name: "Sporting Goods", Slug: "sporting-goods", Icon: "Dumbbell", Order: 12},
		{Name: "Toys & Games", Slug: "toys", Icon: "Gamepad", Order: 13},
	}

	for _, cat := range categories {
		cat.CreatedAt = time.Now()
		// Ideally, we'd check if it exists by slug before inserting, or use upsert
		// The EnsureCategory method in repo should handle this logic
		if err := s.repo.EnsureCategory(ctx, &cat); err != nil {
			log.Printf("Failed to seed category %s: %v", cat.Name, err)
			return err
		}
	}

	log.Println("Marketplace categories seeded successfully.")
	return nil
}
