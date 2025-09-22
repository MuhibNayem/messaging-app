# API Documentation

This document outlines the RESTful API endpoints for the messaging application, covering Messages, Conversations, Friendships, and Groups.

---

## 1. Messages API

### 1.1 Send a Message
- **Summary:** Send a direct or group message.
- **Method:** `POST`
- **Endpoint:** `/messages`
- **Authentication:** `ApiKeyAuth`
- **Request Payload (`models.MessageRequest`):**
  ```json
  {
    "receiver_id": "string", // Optional, for direct messages
    "group_id": "string",    // Optional, for group messages
    "content": "string",     // Optional, if media_urls are provided
    "content_type": "string", // e.g., "text", "image", "video", "file", "audio", "text_image", "text_video", "text_file", "multiple"
    "media_urls": ["string"], // Optional, URLs of media files
    "reply_to_message_id": "string" // Optional, ID of the message being replied to
  }
  ```
  *Note: Either `receiver_id` or `group_id` must be provided, but not both. `content` or `media_urls` must be provided.*
- **Success Response (201 `models.Message`):**
  ```json
  {
    "id": "string",
    "sender_id": "string",
    "sender_name": "string",
    "receiver_id": "string",
    "group_id": "string",
    "group_name": "string",
    "content": "string",
    "content_type": "string",
    "media_urls": ["string"],
    "seen_by": ["string"],
    "is_deleted": false,
    "deleted_at": "timestamp",
    "is_edited": false,
    "edited_at": "timestamp",
    "reactions": [
      {
        "user_id": "string",
        "emoji": "string",
        "timestamp": "timestamp"
      }
    ],
    "reply_to_message_id": "string",
    "created_at": "timestamp",
    "updated_at": "timestamp"
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Invalid user/group ID, missing content/media, invalid content type, both receiver/group ID provided.
  - `403 Forbidden`: Not a group member, can only message friends.
  - `500 Internal Server Error`

### 1.2 Get Messages
- **Summary:** Get messages for a conversation or group.
- **Method:** `GET`
- **Endpoint:** `/messages`
- **Authentication:** `ApiKeyAuth`
- **Query Parameters:**
  - `groupID` (string, optional): Group ID
  - `receiverID` (string, optional): Receiver ID (for direct messages)
  - `page` (int, optional, default: 1): Page number
  - `limit` (int, optional, default: 50): Messages per page (max 100)
  - `before` (string, optional): Get messages before this timestamp (RFC3339)
  *Note: Either `groupID` or `receiverID` must be provided, but not both.*
- **Success Response (200 `models.MessageResponse`):**
  ```json
  {
    "messages": [
      {
        "id": "string",
        "sender_id": "string",
        "sender_name": "string",
        "receiver_id": "string",
        "group_id": "string",
        "group_name": "string",
        "content": "string",
        "content_type": "string",
        "media_urls": ["string"],
        "seen_by": ["string"],
        "is_deleted": false,
        "deleted_at": "timestamp",
        "is_edited": false,
        "edited_at": "timestamp",
        "reactions": [],
        "reply_to_message_id": "string",
        "created_at": "timestamp",
        "updated_at": "timestamp"
      }
    ],
    "total": 0,
    "page": 0,
    "limit": 0,
    "has_more": false
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Both groupID and receiverID provided, neither provided.
  - `500 Internal Server Error`

### 1.3 Mark Messages as Seen
- **Summary:** Mark messages as seen by the current user.
- **Method:** `POST`
- **Endpoint:** `/messages/seen`
- **Authentication:** `ApiKeyAuth`
- **Request Payload:** `array` of `string` (message IDs)
  ```json
  ["message_id_1", "message_id_2"]
  ```
- **Success Response (200 `models.SuccessResponse`):**
  ```json
  {
    "success": true
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Invalid message IDs, empty array.
  - `500 Internal Server Error`

### 1.4 Get Unread Message Count
- **Summary:** Get count of unread messages for the current user.
- **Method:** `GET`
- **Endpoint:** `/messages/unread`
- **Authentication:** `ApiKeyAuth`
- **Success Response (200 `models.UnreadCountResponse`):**
  ```json
  {
    "count": 0
  }
  ```
- **Failure Responses:**
  - `500 Internal Server Error`

### 1.5 Delete a Message
- **Summary:** Delete a message (only for sender or admin).
- **Method:** `DELETE`
- **Endpoint:** `/messages/{id}`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `id` (string, required): Message ID
- **Success Response (200 `models.SuccessResponse`):**
  ```json
  {
    "success": true
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Invalid message ID.
  - `403 Forbidden`
  - `404 Not Found`: Message not found.
  - `500 Internal Server Error`

### 1.6 Edit a Message
- **Summary:** Edit the content of an existing message.
- **Method:** `PUT`
- **Endpoint:** `/messages/{id}`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `id` (string, required): Message ID
- **Request Payload:**
  ```json
  {
    "content": "string" // New message content
  }
  ```
- **Success Response (200 `models.Message`):** (Same as Send Message success response)
- **Failure Responses:**
  - `400 Bad Request`: Missing content, invalid ID format.
  - `403 Forbidden`
  - `404 Not Found`: Message not found, not owned by user, or already deleted.
  - `500 Internal Server Error`

### 1.7 Search Messages
- **Summary:** Search messages in user's conversations.
- **Method:** `GET`
- **Endpoint:** `/messages/search`
- **Authentication:** `ApiKeyAuth`
- **Query Parameters:**
  - `q` (string, required): Search query
  - `page` (int, optional, default: 1): Page number
  - `limit` (int, optional, default: 20): Messages per page
- **Success Response (200 `array` of `models.Message`):** (Array of messages, same structure as `models.Message` in Send Message)
- **Failure Responses:**
  - `400 Bad Request`: Missing search query.
  - `500 Internal Server Error`

### 1.8 Add Reaction to a Message
- **Summary:** Add an emoji reaction to a specific message.
- **Method:** `POST`
- **Endpoint:** `/messages/{id}/react`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `id` (string, required): Message ID
- **Request Payload:**
  ```json
  {
    "emoji": "string" // e.g., "👍", "❤️"
  }
  ```
- **Success Response (200 `models.SuccessResponse`):**
  ```json
  {
    "success": true
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Missing emoji, invalid ID format.
  - `404 Not Found`
  - `409 Conflict`: Reaction already exists.
  - `500 Internal Server Error`

### 1.9 Remove Reaction from a Message
- **Summary:** Remove an emoji reaction from a specific message.
- **Method:** `DELETE`
- **Endpoint:** `/messages/{id}/react`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `id` (string, required): Message ID
- **Request Payload:**
  ```json
  {
    "emoji": "string" // e.g., "👍", "❤️"
  }
  ```
- **Success Response (200 `models.SuccessResponse`):**
  ```json
  {
    "success": true
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Missing emoji, invalid ID format.
  - `404 Not Found`: Reaction not present.
  - `500 Internal Server Error`

---

## 2. Conversations API

### 2.1 Get Conversation Summaries
- **Summary:** Get a list of all conversations (direct and group) with last message details.
- **Method:** `GET`
- **Endpoint:** `/conversations`
- **Authentication:** `ApiKeyAuth`
- **Success Response (200 `array` of `models.ConversationSummary`):**
  ```json
  [
    {
      "id": "string",
      "name": "string",
      "avatar": "string",
      "is_group": false,
      "last_message_content": "string",
      "last_message_timestamp": "timestamp"
    }
  ]
  ```
- **Failure Responses:**
  - `401 Unauthorized`
  - `500 Internal Server Error`

---

## 3. Friendships API

### 3.1 Send Friend Request
- **Summary:** Send a friend request to another user.
- **Method:** `POST`
- **Endpoint:** `/friendships/requests`
- **Authentication:** `ApiKeyAuth`
- **Request Payload (`friendshipRequest`):**
  ```json
  {
    "receiver_id": "string" // ID of the user to send request to
  }
  ```
- **Success Response (201 `models.Friendship`):**
  ```json
  {
    "id": "string",
    "requester_id": "string",
    "receiver_id": "string",
    "status": "pending", // "pending", "accepted", "rejected", "blocked"
    "created_at": "timestamp",
    "updated_at": "timestamp"
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Invalid user ID, invalid receiver ID.
  - `401 Unauthorized`
  - `409 Conflict`: Cannot friend self, friend request already exists.

### 3.2 Respond to Friend Request
- **Summary:** Accept or reject a friend request.
- **Method:** `POST`
- **Endpoint:** `/friendships/requests/respond/{id}`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `id` (string, required): Friendship ID
- **Request Payload (`friendshipResponse`):**
  ```json
  {
    "friendship_id": "string", // ID of the friendship to respond to
    "accept": true             // true to accept, false to reject
  }
  ```
- **Success Response (200 `gin.H`):**
  ```json
  {
    "status": "success"
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Invalid user ID, invalid friendship ID.
  - `401 Unauthorized`
  - `403 Forbidden`: Not authorized.
  - `404 Not Found`: Friend request not found.

### 3.3 List Friendships
- **Summary:** Get a list of friendships with optional status filter.
- **Method:** `GET`
- **Endpoint:** `/friendships`
- **Authentication:** `ApiKeyAuth`
- **Query Parameters:**
  - `status` (string, optional): Friendship status ("pending", "accepted", "rejected")
  - `page` (int, optional, default: 1): Page number
  - `limit` (int, optional, default: 10): Items per page
- **Success Response (200 `gin.H`):**
  ```json
  {
    "data": [
      {
        "id": "string",
        "requester_id": "string",
        "receiver_id": "string",
        "requester_info": {
          "id": "string",
          "username": "string",
          "email": "string",
          "avatar": "string",
          "full_name": "string",
          "bio": "string",
          "date_of_birth": "timestamp",
          "gender": "string",
          "location": "string",
          "phone_number": "string",
          "friends": ["string"],
          "two_factor_enabled": false,
          "email_verified": false,
          "is_active": false,
          "last_login": "timestamp",
          "created_at": "timestamp"
        },
        "receiver_info": {
          "id": "string",
          "username": "string",
          "email": "string",
          "avatar": "string",
          "full_name": "string",
          "bio": "string",
          "date_of_birth": "timestamp",
          "gender": "string",
          "location": "string",
          "phone_number": "string",
          "friends": ["string"],
          "two_factor_enabled": false,
          "email_verified": false,
          "is_active": false,
          "last_login": "timestamp",
          "created_at": "timestamp"
        },
        "status": "string",
        "created_at": "timestamp",
        "updated_at": "timestamp"
      }
    ],
    "total": 0,
    "page": 0,
    "totalPages": 0
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`
  - `401 Unauthorized`

### 3.4 Get Detailed Friendship Status
- **Summary:** Get detailed friendship status between the current user and another user.
- **Method:** `GET`
- **Endpoint:** `/friendships/check`
- **Authentication:** `ApiKeyAuth`
- **Query Parameters:**
  - `other_user_id` (string, required): Other user ID to check
- **Success Response (200 `services.FriendshipStatusResponse`):**
  ```json
  {
    "are_friends": false,
    "request_sent": false,
    "request_received": false,
    "is_blocked_by_viewer": false,
    "has_blocked_viewer": false
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Invalid user IDs.
  - `401 Unauthorized`
  - `500 Internal Server Error`

### 3.5 Unfriend a User
- **Summary:** Remove a friendship between two users.
- **Method:** `DELETE`
- **Endpoint:** `/friendships/{friend_id}`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `friend_id` (string, required): Friend ID to unfriend
- **Success Response (200 `gin.H`):**
  ```json
  {
    "status": "success"
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Invalid user ID, invalid friend ID.
  - `401 Unauthorized`
  - `404 Not Found`: Not friends.

### 3.6 Block a User
- **Summary:** Block another user.
- **Method:** `POST`
- **Endpoint:** `/friendships/block/{user_id}`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `user_id` (string, required): User ID to block
- **Success Response (200 `gin.H`):**
  ```json
  {
    "status": "success"
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Cannot block self, invalid user ID.
  - `401 Unauthorized`
  - `409 Conflict`: Already blocked.

### 3.7 Unblock a User
- **Summary:** Remove a block between users.
- **Method:** `DELETE`
- **Endpoint:** `/friendships/block/{user_id}`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `user_id` (string, required): User ID to unblock
- **Success Response (200 `gin.H`):**
  ```json
  {
    "status": "success"
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Invalid user ID.
  - `401 Unauthorized`
  - `404 Not Found`: Block not found.

### 3.8 Check if User is Blocked
- **Summary:** Check if a user is blocked by another user.
- **Method:** `GET`
- **Endpoint:** `/friendships/block/{user_id}/status`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `user_id` (string, required): User ID to check block status
- **Success Response (200 `gin.H`):**
  ```json
  {
    "is_blocked": false
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Invalid user IDs.
  - `401 Unauthorized`

### 3.9 Get Blocked Users List
- **Summary:** Get a list of users blocked by the current user.
- **Method:** `GET`
- **Endpoint:** `/friendships/blocked`
- **Authentication:** `ApiKeyAuth`
- **Success Response (200 `gin.H`):**
  ```json
  {
    "blocked_users": [
      {
        "id": "string",
        "username": "string",
        "email": "string"
      }
    ]
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`
  - `401 Unauthorized`

---

## 4. Groups API

### 4.1 Create Group
- **Summary:** Create a new group.
- **Method:** `POST`
- **Endpoint:** `/groups`
- **Authentication:** `ApiKeyAuth`
- **Request Payload (`CreateGroupRequest`):**
  ```json
  {
    "name": "string",      // Group name (min 3, max 50 characters)
    "member_ids": ["string"] // Array of user IDs to include as initial members
  }
  ```
- **Success Response (201 `GroupResponse`):**
  ```json
  {
    "id": "string",
    "name": "string",
    "creator": {
      "id": "string",
      "username": "string",
      "email": "string"
    },
    "members": [
      {
        "id": "string",
        "username": "string",
        "email": "string"
      }
    ],
    "admins": [
      {
        "id": "string",
        "username": "string",
        "email": "string"
      }
    ],
    "created_at": "timestamp",
    "updated_at": "timestamp"
  }
  ```
- **Failure Responses:**
  - `400 Bad Request`: Invalid name, invalid member IDs.
  - `401 Unauthorized`
  - `500 Internal Server Error`

### 4.2 Get Group Details
- **Summary:** Get group details.
- **Method:** `GET`
- **Endpoint:** `/groups/{id}`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `id` (string, required): Group ID
- **Success Response (200 `GroupResponse`):** (Same as Create Group success response)
- **Failure Responses:**
  - `400 Bad Request`: Invalid group ID.
  - `404 Not Found`: Group not found.
  - `500 Internal Server Error`

### 4.3 Add Member to Group
- **Summary:** Add a member to a group.
- **Method:** `POST`
- **Endpoint:** `/groups/{id}/members`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `id` (string, required): Group ID
- **Request Payload (`AddMemberRequest`):**
  ```json
  {
    "user_id": "string" // ID of the user to add
  }
  ```
- **Success Response (204 No Content)**
- **Failure Responses:**
  - `400 Bad Request`: Invalid group ID, invalid user ID.
  - `401 Unauthorized`
  - `403 Forbidden`: Not authorized to add member.
  - `404 Not Found`: Group or user not found.
  - `409 Conflict`: User already a member.
  - `500 Internal Server Error`

### 4.4 Add Admin to Group
- **Summary:** Add an admin to a group.
- **Method:** `POST`
- **Endpoint:** `/groups/{id}/admins`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `id` (string, required): Group ID
- **Request Payload (`AddMemberRequest`):**
  ```json
  {
    "user_id": "string" // ID of the user to make admin
  }
  ```
- **Success Response (204 No Content)**
- **Failure Responses:**
  - `400 Bad Request`: Invalid group ID, invalid user ID.
  - `401 Unauthorized`
  - `403 Forbidden`: Not authorized to add admin.
  - `404 Not Found`: Group or user not found.
  - `409 Conflict`: User already an admin.
  - `500 Internal Server Error`

### 4.5 Remove Member from Group
- **Summary:** Remove a member from a group.
- **Method:** `DELETE`
- **Endpoint:** `/groups/{id}/members/{user_id}`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `id` (string, required): Group ID
  - `user_id` (string, required): Member ID to remove
- **Success Response (204 No Content)**
- **Failure Responses:**
  - `400 Bad Request`: Invalid group ID, invalid user ID.
  - `401 Unauthorized`
  - `403 Forbidden`: Not authorized to remove member.
  - `404 Not Found`: Group or user not found.
  - `500 Internal Server Error`

### 4.6 Update Group
- **Summary:** Update group details (e.g., name).
- **Method:** `PUT`
- **Endpoint:** `/groups/{id}`
- **Authentication:** `ApiKeyAuth`
- **Path Parameters:**
  - `id` (string, required): Group ID
- **Request Payload (`UpdateGroupRequest`):**
  ```json
  {
    "name": "string" // New group name (min 3, max 50 characters)
  }
  ```
- **Success Response (200 `GroupResponse`):** (Same as Create Group success response)
- **Failure Responses:**
  - `400 Bad Request`: Invalid group ID, no valid fields to update.
  - `401 Unauthorized`
  - `403 Forbidden`: Not authorized to update group.
  - `404 Not Found`: Group not found.
  - `500 Internal Server Error`

### 4.7 Get User's Groups
- **Summary:** Get all groups the current user is a member of.
- **Method:** `GET`
- **Endpoint:** `/groups`
- **Authentication:** `ApiKeyAuth`
- **Success Response (200 `array` of `GroupResponse`):** (Array of GroupResponse, same structure as Create Group success response)
- **Failure Responses:**
  - `401 Unauthorized`
  - `500 Internal Server Error`
