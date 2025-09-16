
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

### `GET /api/users`

List users with pagination.

**Query Parameters:**

*   `page`: Page number
*   `limit`: Number of items per page
*   `search`: Search query

### `GET /api/users/:id`

Get a user's public profile by ID.

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
