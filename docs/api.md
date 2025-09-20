# API Documentation

This document provides a comprehensive overview of the messaging application's RESTful API endpoints. Each endpoint includes its HTTP method, URL, a description, request/response payloads, and authentication requirements.

## 1. Authentication Endpoints (`/api/auth`)

### 1.1 Register User

Registers a new user in the system.

*   **URL:** `/api/auth/register`
*   **Method:** `POST`
*   **Authentication:** None
*   **Request Body:**
    ```json
    {
        "username": "newuser",
        "email": "newuser@example.com",
        "password": "StrongPassword123!"
    }
    ```
*   **Response (201 Created):**
    ```json
    {
        "access_token": "eyJhbGciOiJIUzI1Ni...",
        "refresh_token": "eyJhbGciOiJIUzI1Ni...",
        "user": {
            "id": "654321098765432109876543",
            "username": "newuser",
            "email": "newuser@example.com",
            "avatar": "",
            "full_name": "",
            "bio": "",
            "date_of_birth": null,
            "gender": "",
            "location": "",
            "phone_number": "",
            "friends": [],
            "two_factor_enabled": false,
            "email_verified": false,
            "is_active": true,
            "last_login": null,
            "created_at": "2023-10-27T10:00:00Z"
        }
    }
    ```
*   **Error Response (400 Bad Request):**
    ```json
    {
        "error": "username already exists"
    }
    ```

### 1.2 Login User

Authenticates a user and provides JWT tokens.

*   **URL:** `/api/auth/login`
*   **Method:** `POST`
*   **Authentication:** None
*   **Request Body:**
    ```json
    {
        "email": "user@example.com",
        "password": "UserPassword123"
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "access_token": "eyJhbGciOiJIUzI1Ni...",
        "refresh_token": "eyJhbGciOiJIUzI1Ni...",
        "user": {
            "id": "654321098765432109876543",
            "username": "existinguser",
            "email": "user@example.com",
            "avatar": "",
            "full_name": "",
            "bio": "",
            "date_of_birth": null,
            "gender": "",
            "location": "",
            "phone_number": "",
            "friends": [],
            "two_factor_enabled": false,
            "email_verified": true,
            "is_active": true,
            "last_login": "2023-10-27T10:00:00Z",
            "created_at": "2023-09-01T00:00:00Z"
        }
    }
    ```
*   **Error Response (401 Unauthorized):**
    ```json
    {
        "error": "invalid credentials: please check email"
    }
    ```

### 1.3 Refresh Access Token

Refreshes an expired access token using a valid refresh token.

*   **URL:** `/api/auth/refresh`
*   **Method:** `POST`
*   **Authentication:** Refresh Token in Request Body
*   **Request Body:**
    ```json
    {
        "refresh_token": "eyJhbGciOiJIUzI1Ni..."
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "access_token": "eyJhbGciOiJIUzI1Ni...",
        "refresh_token": "eyJhbGciOiJIUzI1Ni...",
        "user": {
            "id": "654321098765432109876543",
            "username": "existinguser",
            "email": "user@example.com",
            "avatar": "",
            "full_name": "",
            "bio": "",
            "date_of_birth": null,
            "gender": "",
            "location": "",
            "phone_number": "",
            "friends": [],
            "two_factor_enabled": false,
            "email_verified": true,
            "is_active": true,
            "last_login": "2023-10-27T10:00:00Z",
            "created_at": "2023-09-01T00:00:00Z"
        }
    }
    ```
*   **Error Response (401 Unauthorized):**
    ```json
    {
        "error": "invalid refresh token"
    }
    ```

### 1.4 Logout User

Logs out the current user by blacklisting their access token and deleting their refresh token.

*   **URL:** `/api/auth/logout`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Request Body:** None
*   **Response (200 OK):**
    ```json
    {
        "message": "Successfully logged out"
    }
    ```
*   **Error Response (500 Internal Server Error):**
    ```json
    {
        "error": "failed to delete refresh token"
    }
    ```

## 2. User Endpoints (`/api/users`)

### 2.1 Get Current User Profile

Retrieves the profile information of the authenticated user.

*   **URL:** `/api/users/me`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Request Body:** None
*   **Response (200 OK):**
    ```json
    {
        "id": "654321098765432109876543",
        "username": "currentuser",
        "email": "current@example.com",
        "avatar": "https://example.com/avatar.jpg",
        "created_at": "2023-09-01T00:00:00Z",
        "friends": [
            "654321098765432109876544",
            "654321098765432109876545"
        ],
        "blocked": []
    }
    ```
*   **Error Response (404 Not Found):**
    ```json
    {
        "error": "user not found"
    }
    ```

### 2.2 Get User by ID

Retrieves the public profile information of a specific user by their ID.

*   **URL:** `/api/users/:id`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Request Body:** None
*   **Path Parameters:**
    *   `id` (string, required): The ID of the user to retrieve.
*   **Response (200 OK):**
    ```json
    {
        "id": "654321098765432109876544",
        "username": "anotheruser",
        "avatar": "https://example.com/another_avatar.jpg",
        "created_at": "2023-09-02T00:00:00Z"
    }
    ```
*   **Error Response (404 Not Found):**
    ```json
    {
        "error": "user not found"
    }
    ```

### 2.3 Update Current User Profile

Updates the profile information of the authenticated user.

*   **URL:** `/api/users/me`
*   **Method:** `PUT`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    {
        "username": "updatedusername",
        "full_name": "Updated Name",
        "bio": "A new bio for my profile.",
        "avatar": "https://example.com/new_avatar.jpg"
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "id": "654321098765432109876543",
        "username": "updatedusername",
        "email": "current@example.com",
        "avatar": "https://example.com/new_avatar.jpg",
        "full_name": "Updated Name",
        "bio": "A new bio for my profile.",
        "date_of_birth": null,
        "gender": "",
        "location": "",
        "phone_number": "",
        "friends": [],
        "two_factor_enabled": false,
        "email_verified": true,
        "is_active": true,
        "last_login": "2023-10-27T10:00:00Z",
        "created_at": "2023-09-01T00:00:00Z"
    }
    ```
*   **Error Response (400 Bad Request):**
    ```json
    {
        "error": "username already exists"
    }
    ```

### 2.4 List Users

Retrieves a paginated list of users, with optional search functionality.

*   **URL:** `/api/users`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Request Body:** None
*   **Query Parameters:**
    *   `page` (integer, optional): The page number to retrieve (default: 1).
    *   `limit` (integer, optional): The number of users per page (default: 20, max: 100).
    *   `search` (string, optional): A search query to filter users by username or email.
*   **Response (200 OK):**
    ```json
    {
        "users": [
            {
                "id": "654321098765432109876543",
                "username": "userone",
                "email": "userone@example.com",
                "avatar": "",
                "created_at": "2023-09-01T00:00:00Z"
            },
            {
                "id": "654321098765432109876544",
                "username": "usertwo",
                "email": "usertwo@example.com",
                "avatar": "",
                "created_at": "2023-09-02T00:00:00Z"
            }
        ],
        "total": 2,
        "page": 1,
        "limit": 20
    }
    ```
*   **Error Response (400 Bad Request):**
    ```json
    {
        "error": "invalid page number"
    }
    ```

### 2.5 Update User Email

Updates the email address of the authenticated user.

*   **URL:** `/api/users/me/email`
*   **Method:** `PUT`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    {
        "new_email": "newemail@example.com"
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "success": true
    }
    ```
*   **Error Response (400 Bad Request):**
    ```json
    {
        "error": "email already in use by another account"
    }
    ```

### 2.6 Update User Password

Updates the password of the authenticated user.

*   **URL:** `/api/users/me/password`
*   **Method:** `PUT`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    {
        "current_password": "OldPassword123!",
        "new_password": "NewStrongPassword456!"
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "success": true
    }
    ```
*   **Error Response (400 Bad Request):**
    ```json
    {
        "error": "current password is incorrect"
    }
    ```

### 2.7 Toggle Two-Factor Authentication

Enables or disables two-factor authentication for the authenticated user.

*   **URL:** `/api/users/me/2fa`
*   **Method:** `PUT`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    {
        "enabled": true
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "success": true
    }
    ```
*   **Error Response (500 Internal Server Error):**
    ```json
    {
        "error": "failed to toggle two-factor authentication"
    }
    ```

### 2.8 Deactivate User Account

Deactivates the authenticated user's account.

*   **URL:** `/api/users/me/deactivate`
*   **Method:** `PUT`
*   **Authentication:** Bearer Token
*   **Request Body:** None
*   **Response (200 OK):n    ```json
    {
        "success": true
    }
    ```
*   **Error Response (500 Internal Server Error):**
    ```json
    {
        "error": "failed to deactivate account"
    }
    ```

### 2.9 Update User Privacy Settings

Updates the privacy settings for the authenticated user.

*   **URL:** `/api/users/me/privacy`
*   **Method:** `PUT`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    {
        "default_post_privacy": "FRIENDS",
        "can_see_my_friends_list": "ONLY_ME"
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "success": true
    }
    ```
*   **Error Response (500 Internal Server Error):**
    ```json
    {
        "error": "failed to update privacy settings"
    }
    ```

## 3. Friendship Endpoints (`/api/friendships`)

### 3.1 Send Friend Request

Sends a friend request to another user.

*   **URL:** `/api/friendships/requests`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    {
        "receiver_id": "654321098765432109876544"
    }
    ```
*   **Response (201 Created):**
    ```json
    {
        "id": "654321098765432109876546",
        "requester_id": "654321098765432109876543",
        "receiver_id": "654321098765432109876544",
        "status": "pending",
        "created_at": "2023-10-27T10:00:00Z",
        "updated_at": "2023-10-27T10:00:00Z"
    }
    ```
*   **Error Response (409 Conflict):**
    ```json
    {
        "error": "friend request already exists"
    }
    ```

### 3.2 Respond to Friend Request

Accepts or rejects a pending friend request.

*   **URL:** `/api/friendships/requests/:id/respond`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    {
        "friendship_id": "654321098765432109876546",
        "accept": true
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "status": "success"
    }
    ```
*   **Error Response (404 Not Found):**
    ```json
    {
        "error": "friend request not found"
    }
    ```

### 3.3 List Friendships

Retrieves a paginated list of friendships for the authenticated user, with optional status filtering.

*   **URL:** `/api/friendships`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Query Parameters:**
    *   `status` (string, optional): Filter by friendship status (`pending`, `accepted`, `rejected`).
    *   `page` (integer, optional): The page number to retrieve (default: 1).
    *   `limit` (integer, optional): The number of items per page (default: 10).
*   **Response (200 OK):**
    ```json
    {
        "data": [
            {
                "id": "654321098765432109876546",
                "requester_id": "654321098765432109876543",
                "receiver_id": "654321098765432109876544",
                "status": "accepted",
                "created_at": "2023-10-27T10:00:00Z",
                "updated_at": "2023-10-27T10:00:00Z"
            }
        ],
        "total": 1,
        "page": 1,
        "totalPages": 1
    }
    ```

### 3.4 Check Friendship Status

Checks if the authenticated user is friends with another specified user.

*   **URL:** `/api/friendships/check`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Query Parameters:**
    *   `other_user_id` (string, required): The ID of the other user to check friendship status with.
*   **Response (200 OK):**
    ```json
    {
        "are_friends": true
    }
    ```

### 3.5 Unfriend a User

Removes an existing friendship.

*   **URL:** `/api/friendships/:friend_id`
*   **Method:** `DELETE`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `friend_id` (string, required): The ID of the friend to unfriend.
*   **Response (200 OK):**
    ```json
    {
        "status": "success"
    }
    ```
*   **Error Response (404 Not Found):**
    ```json
    {
        "error": "not friends"
    }
    ```

### 3.6 Block a User

Blocks a specified user.

*   **URL:** `/api/friendships/block/:user_id`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `user_id` (string, required): The ID of the user to block.
*   **Response (200 OK):**
    ```json
    {
        "status": "success"
    }
    ```
*   **Error Response (409 Conflict):**
    ```json
    {
        "error": "user already blocked"
    }
    ```

### 3.7 Unblock a User

Unblocks a previously blocked user.

*   **URL:** `/api/friendships/block/:user_id`
*   **Method:** `DELETE`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `user_id` (string, required): The ID of the user to unblock.
*   **Response (200 OK):**
    ```json
    {
        "status": "success"
    }
    ```
*   **Error Response (404 Not Found):**
    ```json
    {
        "error": "block not found"
    }
    ```

### 3.8 Check if User is Blocked

Checks if the authenticated user has blocked another specified user, or if they are blocked by them.

*   **URL:** `/api/friendships/block/:user_id/status`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `user_id` (string, required): The ID of the user to check block status with.
*   **Response (200 OK):**
    ```json
    {
        "is_blocked": true
    }
    ```

### 3.9 Get Blocked Users List

Retrieves a list of users blocked by the authenticated user.

*   **URL:** `/api/friendships/blocked`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Request Body:** None
*   **Response (200 OK):**
    ```json
    {
        "blocked_users": [
            {
                "id": "654321098765432109876547",
                "username": "blockeduser",
                "email": "blocked@example.com",
                "avatar": "",
                "created_at": "2023-09-03T00:00:00Z"
            }
        ]
    }
    ```

## 4. Group Endpoints (`/api/groups`)

### 4.1 Create Group

Creates a new chat group.

*   **URL:** `/api/groups`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    {
        "name": "My Awesome Group",
        "member_ids": [
            "654321098765432109876544",
            "654321098765432109876545"
        ]
    }
    ```
*   **Response (201 Created):**
    ```json
    {
        "id": "654321098765432109876548",
        "name": "My Awesome Group",
        "creator": {
            "id": "654321098765432109876543",
            "username": "currentuser",
            "email": "current@example.com"
        },
        "members": [
            {
                "id": "654321098765432109876543",
                "username": "currentuser",
                "email": "current@example.com"
            },
            {
                "id": "654321098765432109876544",
                "username": "anotheruser",
                "email": "another@example.com"
            },
            {
                "id": "654321098765432109876545",
                "username": "thirduser",
                "email": "third@example.com"
            }
        ],
        "admins": [
            {
                "id": "654321098765432109876543",
                "username": "currentuser",
                "email": "current@example.com"
            }
        ],
        "created_at": "2023-10-27T10:00:00Z",
        "updated_at": "2023-10-27T10:00:00Z"
    }
    ```
*   **Error Response (400 Bad Request):**
    ```json
    {
        "error": "member 654321098765432109876599 not found"
    }
    ```

### 4.2 Get Group Details

Retrieves the details of a specific chat group.

*   **URL:** `/api/groups/:id`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `id` (string, required): The ID of the group to retrieve.
*   **Response (200 OK):**
    ```json
    {
        "id": "654321098765432109876548",
        "name": "My Awesome Group",
        "creator": {
            "id": "654321098765432109876543",
            "username": "currentuser",
            "email": "current@example.com"
        },
        "members": [
            {
                "id": "654321098765432109876543",
                "username": "currentuser",
                "email": "current@example.com"
            },
            {
                "id": "654321098765432109876544",
                "username": "anotheruser",
                "email": "another@example.com"
            }
        ],
        "admins": [
            {
                "id": "654321098765432109876543",
                "username": "currentuser",
                "email": "current@example.com"
            }
        ],
        "created_at": "2023-10-27T10:00:00Z",
        "updated_at": "2023-10-27T10:00:00Z"
    }
    ```
*   **Error Response (404 Not Found):**
    ```json
    {
        "error": "group not found"
    }
    ```

### 4.3 Update Group

Updates the details of a chat group (e.g., name). Only group admins can perform this action.

*   **URL:** `/api/groups/:id`
*   **Method:** `PATCH`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `id` (string, required): The ID of the group to update.
*   **Request Body:**
    ```json
    {
        "name": "My Renamed Group"
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "id": "654321098765432109876548",
        "name": "My Renamed Group",
        "creator": {
            "id": "654321098765432109876543",
            "username": "currentuser",
            "email": "current@example.com"
        },
        "members": [
            {
                "id": "654321098765432109876543",
                "username": "currentuser",
                "email": "current@example.com"
            }
        ],
        "admins": [
            {
                "id": "654321098765432109876543",
                "username": "currentuser",
                "email": "current@example.com"
            }
        ],
        "created_at": "2023-10-27T10:00:00Z",
        "updated_at": "2023-10-27T10:00:00Z"
    }
    ```
*   **Error Response (403 Forbidden):**
    ```json
    {
        "error": "only admins can update group"
    }
    ```

### 4.4 Add Member to Group

Adds a user as a member to a chat group. Only group admins can perform this action.

*   **URL:** `/api/groups/:id/members`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `id` (string, required): The ID of the group.
*   **Request Body:**
    ```json
    {
        "user_id": "654321098765432109876549"
    }
    ```
*   **Response (204 No Content):** (Success, no content returned)
*   **Error Response (403 Forbidden):**
    ```json
    {
        "error": "only admins can add members"
    }
    ```

### 4.5 Remove Member from Group

Removes a member from a chat group. Only group admins can perform this action.

*   **URL:** `/api/groups/:id/members/:userId`
*   **Method:** `DELETE`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `id` (string, required): The ID of the group.
    *   `userId` (string, required): The ID of the user to remove.
*   **Request Body:** None
*   **Response (204 No Content):** (Success, no content returned)
*   **Error Response (403 Forbidden):**
    ```json
    {
        "error": "only admins can remove members"
    }
    ```

### 4.6 Add Admin to Group

Promotes an existing group member to an admin. Only group admins can perform this action.

*   **URL:** `/api/groups/:id/admins`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `id` (string, required): The ID of the group.
*   **Request Body:**
    ```json
    {
        "user_id": "654321098765432109876549"
    }
    ```
*   **Response (204 No Content):** (Success, no content returned)
*   **Error Response (403 Forbidden):**
    ```json
    {
        "error": "only admins can add other admins"
    }
    ```

### 4.7 Get User's Groups

Retrieves a list of all chat groups the authenticated user is a member of.

*   **URL:** `/api/users/me/groups`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Request Body:** None
*   **Response (200 OK):**
    ```json
    [
        {
            "id": "654321098765432109876548",
            "name": "My Awesome Group",
            "creator": {
                "id": "654321098765432109876543",
                "username": "currentuser",
                "email": "current@example.com"
            },
            "members": [
                {
                    "id": "654321098765432109876543",
                    "username": "currentuser",
                    "email": "current@example.com"
                },
                {
                    "id": "654321098765432109876544",
                    "username": "anotheruser",
                    "email": "another@example.com"
                }
            ],
            "admins": [
                {
                    "id": "654321098765432109876543",
                    "username": "currentuser",
                    "email": "current@example.com"
                }
            ],
            "created_at": "2023-10-27T10:00:00Z",
            "updated_at": "2023-10-27T10:00:00Z"
        }
    ]
    ```

## 5. Messaging Endpoints (`/api/messages`)

### 5.1 Send Message

Sends a direct message to a user or a message to a group.

*   **URL:** `/api/messages`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Request Body (Direct Message):**
    ```json
    {
        "receiver_id": "654321098765432109876544",
        "content": "Hello there!",
        "content_type": "text"
    }
    ```
*   **Request Body (Group Message):**
    ```json
    {
        "group_id": "654321098765432109876548",
        "content": "Hello everyone in the group!",
        "content_type": "text",
        "media_urls": ["https://example.com/image.jpg"]
    }
    ```
*   **Response (201 Created):**
    ```json
    {
        "id": "654321098765432109876550",
        "sender_id": "654321098765432109876543",
        "sender_name": "currentuser",
        "receiver_id": "654321098765432109876544",
        "group_id": null,
        "group_name": "",
        "content": "Hello there!",
        "content_type": "text",
        "media_urls": [],
        "seen_by": [],
        "is_deleted": false,
        "deleted_at": null,
        "created_at": "2023-10-27T10:00:00Z",
        "updated_at": "2023-10-27T10:00:00Z"
    }
    ```
*   **Error Response (403 Forbidden):**
    ```json
    {
        "error": "can only message friends"
    }
    ```

### 5.2 Get Messages

Retrieves a paginated list of messages for a direct conversation or a group chat.

*   **URL:** `/api/messages`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Query Parameters (Direct Message):**
    *   `receiverID` (string, required): The ID of the other user in the direct conversation.
    *   `page` (integer, optional): The page number (default: 1).
    *   `limit` (integer, optional): Messages per page (default: 50, max: 100).
    *   `before` (string, optional): RFC3339 timestamp to get messages before this time.
*   **Query Parameters (Group Message):**
    *   `groupID` (string, required): The ID of the group chat.
    *   `page` (integer, optional): The page number (default: 1).
    *   `limit` (integer, optional): Messages per page (default: 50, max: 100).
    *   `before` (string, optional): RFC3339 timestamp to get messages before this time.
*   **Response (200 OK):**
    ```json
    {
        "messages": [
            {
                "id": "654321098765432109876550",
                "sender_id": "654321098765432109876543",
                "sender_name": "currentuser",
                "receiver_id": "654321098765432109876544",
                "content": "Hello there!",
                "content_type": "text",
                "media_urls": [],
                "seen_by": [
                    "654321098765432109876544"
                ],
                "is_deleted": false,
                "deleted_at": null,
                "created_at": "2023-10-27T10:00:00Z",
                "updated_at": "2023-10-27T10:00:00Z"
            }
        ],
        "total": 1,
        "page": 1,
        "limit": 50,
        "has_more": false
    }
    ```
*   **Error Response (400 Bad Request):**
    ```json
    {
        "error": "cannot specify both groupID and receiverID"
    }
    ```

### 5.3 Mark Messages as Seen

Marks a list of messages as seen by the authenticated user.

*   **URL:** `/api/messages/seen`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    [
        "654321098765432109876550",
        "654321098765432109876551"
    ]
    ```
*   **Response (200 OK):**
    ```json
    {
        "success": true
    }
    ```
*   **Error Response (400 Bad Request):**
    ```json
    {
        "error": "at least one message ID required"
    }
    ```

### 5.4 Get Unread Message Count

Retrieves the total count of unread messages for the authenticated user.

*   **URL:** `/api/messages/unread`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Request Body:** None
*   **Response (200 OK):**
    ```json
    {
        "count": 5
    }
    ```
*   **Error Response (500 Internal Server Error):**
    ```json
    {
        "error": "failed to get unread count"
    }
    ```

### 5.5 Delete Message

Deletes a message. Only the sender or an authorized admin can delete a message. This performs a soft delete.

*   **URL:** `/api/messages/:id`
*   **Method:** `DELETE`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `id` (string, required): The ID of the message to delete.
*   **Request Body:** None
*   **Response (200 OK):**
    ```json
    {
        "success": true
    }
    ```
*   **Error Response (403 Forbidden):**
    ```json
    {
        "error": "not authorized to delete this message"
    }
    ```

## 6. Feed Endpoints (`/api/feed`)

### 6.1 Create Post

Creates a new post on the user's feed.

*   **URL:** `/api/feed/posts`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    {
        "content": "This is my new post! #golang #api @anotheruser",
        "media_type": "image",
        "media_url": "https://example.com/post_image.jpg",
        "privacy": "FRIENDS",
        "custom_audience": [],
        "mentions": ["654321098765432109876544"],
        "hashtags": ["golang", "api"]
    }
    ```
*   **Response (201 Created):**
    ```json
    {
        "id": "654321098765432109876552",
        "user_id": "654321098765432109876543",
        "content": "This is my new post! #golang #api @anotheruser",
        "media_type": "image",
        "media_url": "https://example.com/post_image.jpg",
        "privacy": "FRIENDS",
        "custom_audience": [],
        "comments": [],
        "mentions": ["654321098765432109876544"],
        "reaction_counts": {},
        "hashtags": ["golang", "api"],
        "created_at": "2023-10-27T10:00:00Z",
        "updated_at": "2023-10-27T10:00:00Z"
    }
    ```

### 6.2 Get Post by ID

Retrieves a specific post by its ID.

*   **URL:** `/api/feed/posts/:postId`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `postId` (string, required): The ID of the post to retrieve.
*   **Response (200 OK):**
    ```json
    {
        "id": "654321098765432109876552",
        "user_id": "654321098765432109876543",
        "content": "This is my new post! #golang #api @anotheruser",
        "media_type": "image",
        "media_url": "https://example.com/post_image.jpg",
        "privacy": "FRIENDS",
        "custom_audience": [],
        "comments": [],
        "mentions": ["654321098765432109876544"],
        "reaction_counts": {
            "LIKE": 5,
            "LOVE": 2
        },
        "hashtags": ["golang", "api"],
        "created_at": "2023-10-27T10:00:00Z",
        "updated_at": "2023-10-27T10:00:00Z"
    }
    ```
*   **Error Response (404 Not Found):**
    ```json
    {
        "error": "post not found"
    }
    ```

### 6.3 Update Post

Updates an existing post. Only the post owner can update it.

*   **URL:** `/api/feed/posts/:postId`
*   **Method:** `PUT`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `postId` (string, required): The ID of the post to update.
*   **Request Body:**
    ```json
    {
        "content": "Updated post content!",
        "privacy": "PUBLIC"
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "id": "654321098765432109876552",
        "user_id": "654321098765432109876543",
        "content": "Updated post content!",
        "media_type": "image",
        "media_url": "https://example.com/post_image.jpg",
        "privacy": "PUBLIC",
        "custom_audience": [],
        "comments": [],
        "mentions": ["654321098765432109876544"],
        "reaction_counts": {},
        "hashtags": ["golang", "api"],
        "created_at": "2023-10-27T10:00:00Z",
        "updated_at": "2023-10-27T10:05:00Z"
    }
    ```
*   **Error Response (403 Forbidden):**
    ```json
    {
        "error": "unauthorized to update this post"
    }
    ```

### 6.4 Delete Post

Deletes a post. Only the post owner can delete it.

*   **URL:** `/api/feed/posts/:postId`
*   **Method:** `DELETE`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `postId` (string, required): The ID of the post to delete.
*   **Response (200 OK):**
    ```json
    {
        "success": true
    }
    ```
*   **Error Response (403 Forbidden):**
    ```json
    {
        "error": "unauthorized to delete this post"
    }
    ```

### 6.5 Get Posts by Hashtag

Retrieves a paginated list of posts associated with a specific hashtag.

*   **URL:** `/api/feed/hashtags/:hashtag/posts`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `hashtag` (string, required): The hashtag to search for (e.g., `golang`).
*   **Query Parameters:**
    *   `page` (integer, optional): The page number (default: 1).
    *   `limit` (integer, optional): Posts per page (default: 20).
*   **Response (200 OK):**
    ```json
    {
        "posts": [
            {
                "id": "654321098765432109876552",
                "user_id": "654321098765432109876543",
                "content": "This is my new post! #golang #api",
                "media_type": "",
                "media_url": "",
                "privacy": "PUBLIC",
                "custom_audience": [],
                "comments": [],
                "mentions": [],
                "reaction_counts": {},
                "hashtags": ["golang", "api"],
                "created_at": "2023-10-27T10:00:00Z",
                "updated_at": "2023-10-27T10:00:00Z"
            }
        ],
        "total": 1,
        "page": 1,
        "limit": 20
    }
    ```

### 6.6 Get Comments by Post ID

Retrieves a paginated list of comments for a specific post.

*   **URL:** `/api/feed/posts/:postId/comments`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `postId` (string, required): The ID of the post.
*   **Query Parameters:**
    *   `page` (integer, optional): The page number (default: 1).
    *   `limit` (integer, optional): Comments per page (default: 20).
*   **Response (200 OK):**
    ```json
    [
        {
            "id": "654321098765432109876553",
            "post_id": "654321098765432109876552",
            "user_id": "654321098765432109876544",
            "content": "Great post!",
            "replies": [],
            "reaction_counts": {},
            "mentions": [],
            "created_at": "2023-10-27T10:01:00Z",
            "updated_at": "2023-10-27T10:01:00Z"
        }
    ]
    ```

### 6.7 Create Comment

Creates a new comment on a post.

*   **URL:** `/api/feed/comments`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    {
        "post_id": "654321098765432109876552",
        "content": "My comment on this post! @anotheruser",
        "mentions": ["654321098765432109876544"]
    }
    ```
*   **Response (201 Created):**
    ```json
    {
        "id": "654321098765432109876553",
        "post_id": "654321098765432109876552",
        "user_id": "654321098765432109876543",
        "content": "My comment on this post! @anotheruser",
        "replies": [],
        "reaction_counts": {},
        "mentions": ["654321098765432109876544"],
        "created_at": "2023-10-27T10:01:00Z",
        "updated_at": "2023-10-27T10:01:00Z"
    }
    ```

### 6.8 Update Comment

Updates an existing comment. Only the comment owner can update it.

*   **URL:** `/api/feed/comments/:commentId`
*   **Method:** `PUT`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `commentId` (string, required): The ID of the comment to update.
*   **Request Body:**
    ```json
    {
        "content": "Updated comment content."
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "id": "654321098765432109876553",
        "post_id": "654321098765432109876552",
        "user_id": "654321098765432109876543",
        "content": "Updated comment content.",
        "replies": [],
        "reaction_counts": {},
        "mentions": [],
        "created_at": "2023-10-27T10:01:00Z",
        "updated_at": "2023-10-27T10:02:00Z"
    }
    ```

### 6.9 Delete Comment

Deletes a comment. Only the comment owner can delete it.

*   **URL:** `/api/feed/posts/:postId/comments/:commentId`
*   **Method:** `DELETE`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `postId` (string, required): The ID of the post the comment belongs to.
    *   `commentId` (string, required): The ID of the comment to delete.
*   **Response (200 OK):**
    ```json
    {
        "success": true
    }
    ```

### 6.10 Get Replies by Comment ID

Retrieves a paginated list of replies for a specific comment.

*   **URL:** `/api/feed/comments/:commentId/replies`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `commentId` (string, required): The ID of the comment.
*   **Query Parameters:**
    *   `page` (integer, optional): The page number (default: 1).
    *   `limit` (integer, optional): Replies per page (default: 20).
*   **Response (200 OK):**
    ```json
    [
        {
            "id": "654321098765432109876554",
            "comment_id": "654321098765432109876553",
            "user_id": "654321098765432109876545",
            "content": "My reply to this comment!",
            "reaction_counts": {},
            "mentions": [],
            "created_at": "2023-10-27T10:03:00Z",
            "updated_at": "2023-10-27T10:03:00Z"
        }
    ]
    ```

### 6.11 Create Reply

Creates a new reply to a comment.

*   **URL:** `/api/feed/comments/:commentId/replies`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `commentId` (string, required): The ID of the comment to reply to.
*   **Request Body:**
    ```json
    {
        "content": "My reply to this comment! @thirduser",
        "mentions": ["654321098765432109876545"]
    }
    ```
*   **Response (201 Created):**
    ```json
    {
        "id": "654321098765432109876554",
        "comment_id": "654321098765432109876553",
        "user_id": "654321098765432109876543",
        "content": "My reply to this comment! @thirduser",
        "reaction_counts": {},
        "mentions": ["654321098765432109876545"],
        "created_at": "2023-10-27T10:03:00Z",
        "updated_at": "2023-10-27T10:03:00Z"
    }
    ```

### 6.12 Update Reply

Updates an existing reply. Only the reply owner can update it.

*   **URL:** `/api/feed/comments/:commentId/replies/:replyId`
*   **Method:** `PUT`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `commentId` (string, required): The ID of the comment the reply belongs to.
    *   `replyId` (string, required): The ID of the reply to update.
*   **Request Body:**
    ```json
    {
        "content": "Updated reply content."
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "id": "654321098765432109876554",
        "comment_id": "654321098765432109876553",
        "user_id": "654321098765432109876543",
        "content": "Updated reply content.",
        "reaction_counts": {},
        "mentions": [],
        "created_at": "2023-10-27T10:03:00Z",
        "updated_at": "2023-10-27T10:04:00Z"
    }
    ```

### 6.13 Delete Reply

Deletes a reply. Only the reply owner can delete it.

*   **URL:** `/api/feed/comments/:commentId/replies/:replyId`
*   **Method:** `DELETE`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `commentId` (string, required): The ID of the comment the reply belongs to.
    *   `replyId` (string, required): The ID of the reply to delete.
*   **Response (200 OK):**
    ```json
    {
        "success": true
    }
    ```

### 6.14 Create Reaction

Creates a new reaction on a post, comment, or reply.

*   **URL:** `/api/feed/reactions`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    {
        "target_id": "654321098765432109876552",
        "target_type": "post",
        "type": "LIKE"
    }
    ```
*   **Response (201 Created):**
    ```json
    {
        "id": "654321098765432109876555",
        "user_id": "654321098765432109876543",
        "target_id": "654321098765432109876552",
        "target_type": "post",
        "type": "LIKE",
        "created_at": "2023-10-27T10:05:00Z"
    }
    ```

### 6.15 Delete Reaction

Deletes a reaction. Only the user who created the reaction can delete it.

*   **URL:** `/api/feed/reactions/:reactionId`
*   **Method:** `DELETE`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `reactionId` (string, required): The ID of the reaction to delete.
*   **Query Parameters:**
    *   `targetId` (string, required): The ID of the target (post, comment, or reply) the reaction belongs to.
    *   `targetType` (string, required): The type of the target (`post`, `comment`, or `reply`).
*   **Response (200 OK):**
    ```json
    {
        "success": true
    }
    ```

### 6.16 Get Reactions by Post ID

Retrieves a list of reactions for a specific post.

*   **URL:** `/api/feed/posts/:postId/reactions`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `postId` (string, required): The ID of the post.
*   **Response (200 OK):**
    ```json
    [
        {
            "id": "654321098765432109876555",
            "user_id": "654321098765432109876543",
            "target_id": "654321098765432109876552",
            "target_type": "post",
            "type": "LIKE",
            "created_at": "2023-10-27T10:05:00Z"
        }
    ]
    ```

### 6.17 Get Reactions by Comment ID

Retrieves a list of reactions for a specific comment.

*   **URL:** `/api/feed/comments/:commentId/reactions`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `commentId` (string, required): The ID of the comment.
*   **Response (200 OK):**
    ```json
    [
        {
            "id": "654321098765432109876556",
            "user_id": "654321098765432109876544",
            "target_id": "654321098765432109876553",
            "target_type": "comment",
            "type": "LOVE",
            "created_at": "2023-10-27T10:06:00Z"
        }
    ]
    ```

### 6.18 Get Reactions by Reply ID

Retrieves a list of reactions for a specific reply.

*   **URL:** `/api/feed/replies/:replyId/reactions`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `replyId` (string, required): The ID of the reply.
*   **Response (200 OK):**
    ```json
    [
        {
            "id": "654321098765432109876557",
            "user_id": "654321098765432109876545",
            "target_id": "654321098765432109876554",
            "target_type": "reply",
            "type": "HAHA",
            "created_at": "2023-10-27T10:07:00Z"
        }
    ]
    ```

## 7. Privacy Endpoints (`/api/privacy`)

### 7.1 Get User Privacy Settings

Retrieves the privacy settings for the authenticated user.

*   **URL:** `/api/privacy/settings`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Request Body:** None
*   **Response (200 OK):**
    ```json
    {
        "user_id": "654321098765432109876543",
        "default_post_privacy": "FRIENDS",
        "can_see_my_friends_list": "ONLY_ME",
        "can_send_me_friend_requests": "EVERYONE",
        "can_tag_me_in_posts": "FRIENDS",
        "last_updated": "2023-10-27T10:00:00Z"
    }
    ```

### 7.2 Update User Privacy Settings

Updates the privacy settings for the authenticated user.

*   **URL:** `/api/privacy/settings`
*   **Method:** `PUT`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    {
        "default_post_privacy": "PUBLIC",
        "can_see_my_friends_list": "FRIENDS"
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "user_id": "654321098765432109876543",
        "default_post_privacy": "PUBLIC",
        "can_see_my_friends_list": "FRIENDS",
        "can_send_me_friend_requests": "EVERYONE",
        "can_tag_me_in_posts": "FRIENDS",
        "last_updated": "2023-10-27T10:05:00Z"
    }
    ```

### 7.3 Create Custom Privacy List

Creates a new custom privacy list for the authenticated user.

*   **URL:** `/api/privacy/lists`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Request Body:**
    ```json
    {
        "name": "Close Friends",
        "members": [
            "654321098765432109876544",
            "654321098765432109876545"
        ]
    }
    ```
*   **Response (201 Created):**
    ```json
    {
        "id": "654321098765432109876558",
        "user_id": "654321098765432109876543",
        "name": "Close Friends",
        "members": [
            "654321098765432109876544",
            "654321098765432109876545"
        ],
        "created_at": "2023-10-27T10:00:00Z",
        "updated_at": "2023-10-27T10:00:00Z"
    }
    ```

### 7.4 Get Custom Privacy List by ID

Retrieves a specific custom privacy list by its ID.

*   **URL:** `/api/privacy/lists/:id`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `id` (string, required): The ID of the custom privacy list.
*   **Response (200 OK):**
    ```json
    {
        "id": "654321098765432109876558",
        "user_id": "654321098765432109876543",
        "name": "Close Friends",
        "members": [
            "654321098765432109876544",
            "654321098765432109876545"
        ],
        "created_at": "2023-10-27T10:00:00Z",
        "updated_at": "2023-10-27T10:00:00Z"
    }
    ```
*   **Error Response (404 Not Found):**
    ```json
    {
        "error": "custom privacy list not found"
    }
    ```

### 7.5 Get Custom Privacy Lists by User ID

Retrieves all custom privacy lists created by the authenticated user.

*   **URL:** `/api/privacy/lists`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Request Body:** None
*   **Response (200 OK):**
    ```json
    [
        {
            "id": "654321098765432109876558",
            "user_id": "654321098765432109876543",
            "name": "Close Friends",
            "members": [
                "654321098765432109876544",
                "654321098765432109876545"
            ],
            "created_at": "2023-10-27T10:00:00Z",
            "updated_at": "2023-10-27T10:00:00Z"
        }
    ]
    ```

### 7.6 Update Custom Privacy List

Updates an existing custom privacy list. Only the list owner can update it.

*   **URL:** `/api/privacy/lists/:id`
*   **Method:** `PUT`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `id` (string, required): The ID of the custom privacy list to update.
*   **Request Body:**
    ```json
    {
        "name": "Super Close Friends",
        "members": [
            "654321098765432109876544"
        ]
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "id": "654321098765432109876558",
        "user_id": "654321098765432109876543",
        "name": "Super Close Friends",
        "members": [
            "654321098765432109876544"
        ],
        "created_at": "2023-10-27T10:00:00Z",
        "updated_at": "2023-10-27T10:10:00Z"
    }
    ```

### 7.7 Delete Custom Privacy List

Deletes a custom privacy list. Only the list owner can delete it.

*   **URL:** `/api/privacy/lists/:id`
*   **Method:** `DELETE`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `id` (string, required): The ID of the custom privacy list to delete.
*   **Response (200 OK):**
    ```json
    {
        "success": true
    }
    ```

### 7.8 Add Member to Custom Privacy List

Adds a member to an existing custom privacy list. Only the list owner can modify it.

*   **URL:** `/api/privacy/lists/:id/members`
*   **Method:** `POST`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `id` (string, required): The ID of the custom privacy list.
*   **Request Body:**
    ```json
    {
        "user_id": "654321098765432109876549"
    }
    ```
*   **Response (200 OK):**
    ```json
    {
        "id": "654321098765432109876558",
        "user_id": "654321098765432109876543",
        "name": "Super Close Friends",
        "members": [
            "654321098765432109876544",
            "654321098765432109876549"
        ],
        "created_at": "2023-10-27T10:00:00Z",
        "updated_at": "2023-10-27T10:15:00Z"
    }
    ```

### 7.9 Remove Member from Custom Privacy List

Removes a member from an existing custom privacy list. Only the list owner can modify it.

*   **URL:** `/api/privacy/lists/:id/members/:member_id`
*   **Method:** `DELETE`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `id` (string, required): The ID of the custom privacy list.
    *   `member_id` (string, required): The ID of the member user to remove.
*   **Request Body:** None
*   **Response (200 OK):**
    ```json
    {
        "id": "654321098765432109876558",
        "user_id": "654321098765432109876543",
        "name": "Super Close Friends",
        "members": [
            "654321098765432109876544"
        ],
        "created_at": "2023-10-27T10:00:00Z",
        "updated_at": "2023-10-27T10:20:00Z"
    }
    ```

## 8. Notification Endpoints (`/api/notifications`)

### 8.1 List Notifications

Retrieves a paginated list of notifications for the authenticated user.

*   **URL:** `/api/notifications`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Query Parameters:**
    *   `page` (integer, optional): The page number to retrieve (default: 1).
    *   `limit` (integer, optional): The number of notifications per page (default: 10).
    *   `read` (boolean, optional): Filter by read status (true for read, false for unread).
*   **Response (200 OK):**
    ```json
    {
        "notifications": [
            {
                "id": "654321098765432109876559",
                "recipient_id": "654321098765432109876543",
                "sender_id": "654321098765432109876544",
                "type": "LIKE",
                "target_id": "654321098765432109876552",
                "target_type": "post",
                "content": "anotheruser liked your post.",
                "data": {},
                "read": false,
                "created_at": "2023-10-27T10:00:00Z"
            }
        ],
        "total": 1,
        "page": 1,
        "limit": 10
    }
    ```

### 8.2 Mark Notification as Read

Marks a specific notification as read for the authenticated user.

*   **URL:** `/api/notifications/:id/read`
*   **Method:** `PUT`
*   **Authentication:** Bearer Token
*   **Path Parameters:**
    *   `id` (string, required): The ID of the notification to mark as read.
*   **Response (200 OK):**
    ```json
    {
        "success": true
    }
    ```
*   **Error Response (404 Not Found):**
    ```json
    {
        "error": "notification not found"
    }
    ```
*   **Error Response (403 Forbidden):**
    ```json
    {
        "error": "unauthorized to mark this notification as read"
    }
    ```

### 8.3 Get Unread Notification Count

Retrieves the total count of unread notifications for the authenticated user.

*   **URL:** `/api/notifications/unread`
*   **Method:** `GET`
*   **Authentication:** Bearer Token
*   **Response (200 OK):**
    ```json
    {
        "count": 5
    }
    ```
*   **Error Response (500 Internal Server Error):**
    ```json
    {
        "error": "failed to get unread count"
    }
    ```
