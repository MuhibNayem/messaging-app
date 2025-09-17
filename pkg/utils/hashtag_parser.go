package utils

import (
	"regexp"
	"strings"
)

// ExtractHashtags finds all hashtags (words prefixed with #) in a given text.
// It returns a slice of unique hashtags without the '#' prefix, converted to lowercase.
func ExtractHashtags(text string) []string {
	// Regular expression to find words starting with #
	// It captures the word characters after the #
	re := regexp.MustCompile(`#(\w+)`)
	matches := re.FindAllStringSubmatch(text, -1)

	uniqueHashtags := make(map[string]struct{})
	var hashtags []string

	for _, match := range matches {
		if len(match) > 1 {
			hashtag := strings.ToLower(match[1]) // Convert to lowercase and remove '#'
			if _, exists := uniqueHashtags[hashtag]; !exists {
				uniqueHashtags[hashtag] = struct{}{}
				hashtags = append(hashtags, hashtag)
			}
		}
	}
	return hashtags
}
