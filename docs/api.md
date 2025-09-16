# API Documentation

This document provides detailed information about the messaging application's API endpoints.

## Authentication

### `POST /api/auth/register`

Registers a new user.

**Request Body:**

```json
{
  "username": "testuser",
  "email": "test@example.com",
  "password": "password123"
}
```

**Response:**

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "user": {
    "id": "...",
    "username": "testuser",
    "email": "test@example.com",
    "avatar": "",
    "friends": [],
    "created_at": "..."
  }
}
```

### `POST /api/auth/login`

Logs in an existing user.

**Request Body:**

```json
{
  "email": "test@example.com",
  "password": "password123"
}
```

**Response:**

Same as registration response.

### `POST /api/auth/refresh`

Refreshes the access token.

**Request Body:**

```json
{
  "refresh_token": "..."
}
```

**Response:**

Same as registration response.

### `POST /api/auth/logout`

Logs out the user and invalidates tokens.

**Response:**

```json
{
  "message": "Successfully logged out"
}
```

## Users

### `GET /api/user`

Get the current user's profile.

### `PUT /api/user`

Update the current user's profile.

**Request Body:**

```json
{
  "username": "newusername"
}
```

### `PUT /api/user/email`

Update the current user's email address.

**Request Body:**

```json
{
  "new_email": "new@example.com"
}
```

**Response:**

```json
{
  "success": true
}
```

### `PUT /api/user/password`

Update the current user's password.

**Request Body:**

```json
{
  "current_password": "oldpassword",
  "new_password": "newpassword123"
}
```

**Response:**

```json
{
  "success": true
}
```

### `PUT /api/user/2fa`

Enable or disable two-factor authentication for the current user.

**Request Body:**

```json
{
  "enabled": true
}
```

**Response:**

```json
{
  "success": true
}
```

### `PUT /api/user/deactivate`

Deactivate the current user's account.

**Response:**

```json
{
  "success": true
}
```

### `GET /api/users`

List users with pagination.

**Query Parameters:**

*   `page`: Page number
*   `limit`: Number of items per page
*   `search`: Search query

### `GET /api/users/:id`

Get a user's public profile by ID.

## User Privacy Settings

### `PUT /api/user/privacy`

Update the current user's privacy settings.

**Request Body:**

```json
{
  "default_post_privacy": "PUBLIC",
  "can_see_my_friends_list": "FRIENDS",
  "can_send_me_friend_requests": "EVERYONE",
  "can_tag_me_in_posts": "EVERYONE"
}
```

**Response:**

```json
{
  "success": true
}
```

## Feed (Posts)

### `POST /api/posts`

Create a new post.

**Request Body:**

```json
{
  "content": "My first post!",
  "media_type": "image",
  "media_url": "http://example.com/image.jpg",
  "privacy": "PUBLIC",
  "custom_audience": ["654c1a00a0b1c2d3e4f5a6b7"],
  "mentions": ["654c1a00a0b1c2d3e4f5a6b8"],
  "hashtags": ["firstpost", "exciting"]
}
```

**Response:**

```json
{
  "id": "...",
  "user_id": "...",
  "content": "My first post!",
  "media_type": "image",
  "media_url": "http://example.com/image.jpg",
  "privacy": "PUBLIC",
  "custom_audience": ["654c1a00a0b1c2d3e4f5a6b7"],
  "likes": [],
  "comments": [],
  "mentions": ["654c1a00a0b1c2d3e4f5a6b8"],
  "hashtags": ["firstpost", "exciting"],
  "created_at": "...",
  "updated_at": "..."
}
```

### `GET /api/posts/{id}`

Get a post by ID.

**Response:**

```json
{
  "id": "...",
  "user_id": "...",
  "content": "My first post!",
  "media_type": "image",
  "media_url": "http://example.com/image.jpg",
  "privacy": "PUBLIC",
  "custom_audience": ["654c1a00a0b1c2d3e4f5a6b7"],
  "likes": ["654c1a00a0b1c2d3e4f5a6b9"],
  "comments": [],
  "mentions": ["654c1a00a0b1c2d3e4f5a6b8"],
  "hashtags": ["firstpost", "exciting"],
  "created_at": "...",
  "updated_at": "..."
}
```

### `PUT /api/posts/{id}`

Update an existing post.

**Request Body:**

```json
{
  "content": "Updated content!",
  "privacy": "FRIENDS"
}
```

**Response:**

Same as `GET /api/posts/{id}`.

### `DELETE /api/posts/{id}`

Delete a post.

**Response:**

```json
{
  "success": true
}
```

### `GET /api/posts/{postId}/comments`

Get comments for a specific post.

**Query Parameters:**

*   `page`: Page number
*   `limit`: Number of items per page

**Response:**

```json
[
  {
    "id": "...",
    "post_id": "...",
    "user_id": "...",
    "content": "Great post!",
    "likes": [],
    "replies": [],
    "mentions": [],
    "created_at": "...",
    "updated_at": "..."
  }
]
```

### `GET /api/posts/{postId}/reactions`

Get reactions for a specific post.

**Response:**

```json
[
  {
    "id": "...",
    "user_id": "...",
    "target_id": "...",
    "target_type": "post",
    "type": "LIKE",
    "created_at": "..."
  }
]
```

## Feed (Comments)

### `POST /api/comments`

Create a new comment on a post.

**Request Body:**

```json
{
  "post_id": "654c1a00a0b1c2d3e4f5a6b7",
  "content": "This is a comment.",
  "mentions": ["654c1a00a0b1c2d3e4f5a6b8"]
}
```

**Response:**

```json
{
  "id": "...",
  "post_id": "...",
  "user_id": "...",
  "content": "This is a comment.",
  "likes": [],
  "replies": [],
  "mentions": ["654c1a00a0b1c2d3e4f5a6b8"],
  "created_at": "...",
  "updated_at": "..."
}
```

### `PUT /api/comments/{id}`

Update an existing comment.

**Request Body:**

```json
{
  "content": "Updated comment content."
}
```

**Response:**

Same as `POST /api/comments`.

### `DELETE /api/posts/{postId}/comments/{commentId}`

Delete a comment.

**Response:**

```json
{
  "success": true
}
```

### `GET /api/comments/{commentId}/replies`

Get replies for a specific comment.

**Query Parameters:**

*   `page`: Page number
*   `limit`: Number of items per page

**Response:**

```json
[
  {
    "id": "...",
    "comment_id": "...",
    "user_id": "...",
    "content": "This is a reply.",
    "likes": [],
    "mentions": [],
    "created_at": "...",
    "updated_at": "..."
  }
]
```

### `GET /api/comments/{commentId}/reactions`

Get reactions for a specific comment.

**Response:**

```json
[
  {
    "id": "...",
    "user_id": "...",
    "target_id": "...",
    "target_type": "comment",
    "type": "LOVE",
    "created_at": "..."
  }
]
```

## Feed (Replies)

### `POST /api/comments/{commentId}/replies`

Create a new reply to a comment.

**Request Body:**

```json
{
  "content": "This is a reply to the comment.",
  "mentions": ["654c1a00a0b1c2d3e4f5a6b9"]
}
```

**Response:**

```json
{
  "id": "...",
  "comment_id": "...",
  "user_id": "...",
  "content": "This is a reply to the comment.",
  "likes": [],
  "mentions": ["654c1a00a0b1c2d3e4f5a6b9"],
  "created_at": "...",
  "updated_at": "..."
}
```

### `PUT /api/comments/{commentId}/replies/{replyId}`

Update an existing reply.

**Request Body:**

```json
{
  "content": "Updated reply content."
}
```

**Response:**

Same as `POST /api/comments/{commentId}/replies`.

### `DELETE /api/comments/{commentId}/replies/{replyId}`

Delete a reply.

**Response:**

```json
{
  "success": true
}
```

### `GET /api/replies/{replyId}/reactions`

Get reactions for a specific reply.

**Response:**

```json
[
  {
    "id": "...",
    "user_id": "...",
    "target_id": "...",
    "target_type": "reply",
    "type": "HAHA",
    "created_at": "..."
  }
]
```

## Feed (Reactions)

### `POST /api/reactions`

Create a new reaction on a post, comment, or reply.

**Request Body:**

```json
{
  "target_id": "654c1a00a0b1c2d3e4f5a6b7",
  "target_type": "post",
  "type": "LIKE"
}
```

**Response:**

```json
{
  "id": "...",
  "user_id": "...",
  "target_id": "...",
  "target_type": "post",
  "type": "LIKE",
  "created_at": "..."
}
```

### `DELETE /api/reactions/{reactionId}`

Delete a reaction.

**Query Parameters:**

*   `targetId`: ID of the target (post, comment, or reply)
*   `targetType`: Type of the target ("post", "comment", or "reply")

**Response:**

```json
{
  "success": true
}
```

## Friendship

### `POST /api/friendships/requests`

Send a friend request.

**Request Body:**

```json
{
  "receiver_id": "..."
}
```

### `POST /api/friendships/requests/:id/respond`

Respond to a friend request.

**Request Body:**

```json
{
  "friendship_id": "...",
  "accept": true
}
```

### `GET /api/friendships`

List friendships.

**Query Parameters:**

*   `status`: `pending`, `accepted`, `rejected`

### `DELETE /api/friendships/:id`

Unfriend a user.

## Groups

### `POST /api/groups`

Create a new group.

**Request Body:**

```json
{
  "name": "My Group",
  "member_ids": ["...", "..."]
}
```

### `GET /api/groups/:id`

Get group details.

### `POST /api/groups/:id/members`

Add a member to a group.

**Request Body:**

```json
{
  "user_id": "..."
}
```

### `DELETE /api/groups/:id/members/:user_id`

Remove a member from a group.

## Messaging

### `POST /api/messages`

Send a message.

**Request Body (Direct Message):**

```json
{
  "receiver_id": "...",
  "content": "Hello!"
}
```

**Request Body (Group Message):**

```json
{
  "group_id": "...",
  "content": "Hello everyone!"
}
```

### `GET /api/messages/:id`

Get messages from a conversation.

**Query Parameters:**

*   `page`: Page number
*   `limit`: Number of items per page

### `DELETE /api/messages/:id`

Delete a message.

## WebSocket

### `GET /ws`

Upgrades the connection to a WebSocket for real-time communication.