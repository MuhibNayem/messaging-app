# Project Development Roadmap

This document outlines the phased development roadmap for the messaging application, detailing expectations, current state, and requirements for each feature.

## Phase 1: Core Engagement & Personalization
*   Focus: Enhancing the user's individual experience, making the feed more relevant, and improving profile expressiveness.

### 1.1 Enhanced Feed Personalization and Content Discovery
  *  **Expectations:**
        *   Users see content most relevant to them, not just chronological.
        *   Increased time spent on the feed due to higher relevance.
        *   Easier discovery of new connections and content.
  *  **Current State:**
        *   Feed is primarily chronological.
        *   Basic hashtag search exists.
        *   No explicit "people you may know" suggestions.
  *  **Requirements:**
        *   **Basic Feed Algorithm:**
            *   Implement a ranking algorithm considering:
                *   Recency of post.
                *   Engagement metrics (likes, comments, shares) on posts.
                *   Relationship strength with poster (friends > non-friends).
                *   User's past interactions (e.g., if user frequently interacts with posts about #golang, prioritize #golang posts).
            *   API endpoint to fetch personalized feed.
        *  **"People You May Know" Suggestions:**
            *   Algorithm to suggest users based on:
                *   Mutual friends.
                *   Shared group memberships.
                *   Interaction history (e.g., commented on same posts).
            *   API endpoint to retrieve suggestions.
            *   UI integration for displaying suggestions (e.g., sidebar, dedicated section).
        *  **Advanced Search:**
            *   Extend search to cover users, posts (by content, hashtags, mentions), and groups.
            *   Implement indexing for efficient full-text search (e.g., using MongoDB Atlas Search or a dedicated search service like Elasticsearch).
            *   API endpoints for each search type.

### 1.2 Richer User Profiles
  *  **Expectations:**
        *   Users can express more about themselves.
        *   Profiles are visually more appealing and informative.
        *   Easier for users to learn about others.
  *  **Current State:**
        *   Basic user profile with username, email, avatar, bio, full name, creation date.
        *   Limited customization options.
  *  **Requirements:**
        *   **Customizable Profile Fields:**
            *   Add fields for: interests, work/education history, personal website/links, preferred pronouns.
            *   Allow users to set privacy for each field (e.g., public, friends, only me).
            *   API endpoints for updating these new fields.
        *   **Profile Customization:**
            *   Ability to upload and set a "cover photo" for the profile.
            *   More diverse avatar options (e.g., custom frames, filters).
            *   API endpoints for managing cover photos and advanced avatar settings.
        *   **Profile UI/UX:**
            *   Design and implement a more dynamic and customizable profile page layout.

## Phase 2: Community & Communication Expansion
*   Focus: Deepening group interactions, enhancing real-time communication, and enabling event management.

### 2.1 Events and Scheduling
  *   **Expectations:**
      *   Users can easily create, manage, and discover social events.
      *   Improved coordination for gatherings.
      *   Increased real-world interaction among users.

  *   **Current State:**
      *   No event management functionality exists.

  *   **Requirements:**
      
      *  **Event Data Model:**   
          *   Define a new `Event` model (name, description, date/time, location, creator, attendees, privacy settings, cover image).

      *   **Event Creation & Management:**
          *   API endpoints for creating, updating, and deleting events.
          *   Ability to invite friends/groups to events.
          *   RSVP functionality (going, interested, declined).
      *   **Event Discovery:**
          *   API endpoint to list upcoming events (personalized based on location, interests, friends' events).
          *   Dedicated UI section for events.
      *   **Notifications:**
          *   Send notifications for event invitations, reminders, and updates.

### 2.2 Advanced Messaging Features
  *   **Expectations:**
        *   More dynamic and expressive real-time chat experience.
        *   Better context and feedback during conversations.
  *   **Current State:**
        *   Basic direct and group messaging.
        *   Messages can contain text and media URLs.
        *   Seen status for messages.
  *   **Requirements:**
        *   **Typing Indicators:**
            *   Real-time WebSocket events to broadcast "user is typing..." status within a conversation.
            *   Client-side implementation to display indicators.
        *   **Read Receipts (Optional):**
            *   Allow users to toggle read receipts on/off in settings.
            *   WebSocket events and API updates to indicate when a message has been read by recipients.
        *   **Rich Media Previews:**
            *   Backend service to fetch metadata (title, description, image) for shared URLs.
            *   Client-side rendering of rich previews for links, images, and videos.
        *   **Stickers and GIFs:**
            *   Integration with third-party services (e.g., Giphy, custom sticker packs).
            *   API endpoints for browsing and sending stickers/GIFs.
            *   Client-side rendering of these media types.

### 2.3 Advanced Group and Community Enhancements
  *   **Expectations:**
        *   More flexible and powerful group management.
        *   Stronger sense of community within groups.
        *   Easier to find and manage groups.
  *   **Current State:**
        *   Basic group creation, member addition/removal, admin promotion.
        *   Groups have a name, creator, members, and admins.
  *   **Requirements:**
        *   **Granular Group Privacy:**
            *   Add "Secret" group type (not discoverable, invite-only).
            *   More detailed privacy settings for group content visibility.
        *   **Group Roles and Permissions:**
            *   Define custom roles (e.g., Moderator) with specific permissions (e.g., delete posts, mute members).
            *   API endpoints for assigning/managing roles and permissions.
        *   **Group Discovery and Recommendations:**
            *   API endpoint to search for public/private groups by name, topic, or member count.
            *   Algorithm to recommend groups based on user interests, friends' memberships.
        *   **Integrated Group Features:**
            *   Ability to create and manage events directly within a group.
            *   Dedicated discussion forums or topic channels within groups.

## Phase 3: Platform Health & Ecosystem
*   Focus: Ensuring a safe environment, establishing monetization, and enabling external integrations.

### 3.1 Robust Content Moderation and Reporting System
  *   **Expectations:**
        *   Users feel safe and protected from harmful content/behavior.
        *   Efficient process for handling reports.
        *   Clear and fair moderation policies.
  *   **Current State:**
        *   No explicit reporting or moderation system.
  *   **Requirements:**
        *   **User Reporting:**
            *   API endpoints for users to report posts, comments, messages, profiles.
            *   `Report` data model (reporter ID, target ID, target type, reason, status).
        *   **Moderation Dashboard (Admin UI):**
            *   Web interface for moderators to view, filter, and act on reports.
            *   Tools for content removal, user warnings, temporary/permanent bans.
            *   Audit trail for all moderation actions.
        *   **Automated Detection (Basic):**
            *   Implement keyword filtering for offensive language.
            *   Basic spam detection (e.g., repetitive content, suspicious links).
        *   **Appeal Process:**
            *   API endpoints for users to appeal moderation decisions.
            *   Integration with moderation dashboard for review of appeals.

### 3.2 Initial Monetization Strategy
  *   **Expectations:**
        *   Platform generates revenue to support development and operations.
        *   Clear value proposition for premium features.
        *   Non-intrusive advertising experience.
  *   **Current State:**
        *   No monetization features.
  *   **Requirements:**
        *   **Premium Features/Subscriptions:**
            *   Define subscription tiers and associated benefits.
            *   Integration with a payment gateway (e.g., Stripe) for subscription management.
            *   API endpoints for managing subscriptions and checking user entitlements.
        *   **Virtual Goods/Gifts:**
            *   `VirtualGood` data model (name, price, type).
            *   API endpoints for purchasing and sending virtual goods.
            *   Integration with payment gateway for one-time purchases.
        *   **Basic Ad Placement:**
            *   Define ad slots within the UI.
            *   Mechanism to serve static or programmatic ads (initially, direct deals or a simple ad network integration).
            *   API endpoints for fetching ad content.

### 3.3 External Integrations and Developer Platform (Foundational)
  *   **Expectations:**
        *   Platform can connect with other services.
        *   Developers can build on top of the platform.
        *   Increased reach and functionality through third-party apps.
  *   **Current State:**
        *   No public API or integration mechanisms.
  *   **Requirements:**
        *   **OAuth 2.0 Provider:**
            *   Implement OAuth 2.0 authorization server for secure third-party app authentication.
            *   API endpoints for authorization, token issuance, and token revocation.
        *   **Basic API for Content Sharing:**
            *   Public API endpoints for authenticated users to post content to their feed or retrieve public data.
            *   Clear documentation for API usage, authentication, and rate limits.
        *   **Webhooks:**
            *   Allow third-party services to register webhooks to receive real-time notifications about specific events (e.g., new post by a user, new message in a public group).
            *   API endpoints for webhook registration and management.