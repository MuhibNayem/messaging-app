package utils

import (
	"regexp"
)

// ExtractMentions extracts usernames mentioned in a text (e.g., @username)
func ExtractMentions(text string) []string {
	// Regular expression to find words starting with @
	// It captures the word following @, ensuring it's not just part of an email or URL
	// and consists of word characters (letters, numbers, underscore)
	re := regexp.MustCompile(`@([\w]+)`)
	matches := re.FindAllStringSubmatch(text, -1)

	var mentions []string
	for _, match := range matches {
		if len(match) > 1 {
			mentions = append(mentions, match[1])
		}
	}
	return mentions
}
