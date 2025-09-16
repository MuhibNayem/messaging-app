# Business Logic and User Stories

This document outlines the core business logic of the messaging application through user stories and scenarios.

## 1. Authentication

### 1.1. User Registration

*   **As a** new user
*   **I want to** create an account
*   **So that** I can start using the messaging application.

**Scenario:**
*   A new user provides their username, email, and password.
*   The system checks if the email or username is already in use.
*   If not, it hashes the password and creates a new user in the database.
*   The system generates and returns JWT access and refresh tokens.

```mermaid
sequenceDiagram
    participant User
    participant API
    participant AuthService
    participant UserRepo
    User->>API: POST /api/auth/register (username, email, password)
    API->>AuthService: Register(user)
    AuthService->>UserRepo: FindUserByEmail(email)
    UserRepo-->>AuthService: (nil)
    AuthService->>UserRepo: FindUserByUserName(username)
    UserRepo-->>AuthService: (nil)
    AuthService->>AuthService: Hash password
    AuthService->>UserRepo: CreateUser(user)
    UserRepo-->>AuthService: (createdUser)
    AuthService->>AuthService: Generate JWT tokens
    AuthService-->>API: (tokens, userDetails)
    API-->>User: 201 Created (tokens, userDetails)
```

### 1.2. User Login

*   **As an** existing user
*   **I want to** log in to my account
*   **So that** I can access my messages and contacts.

**Scenario:**
*   An existing user provides their email and password.
*   The system finds the user by email.
*   It compares the provided password with the stored hash.
*   If the credentials are correct, it generates and returns new JWT access and refresh tokens.

```mermaid
sequenceDiagram
    participant User
    participant API
    participant AuthService
    participant UserRepo
    User->>API: POST /api/auth/login (email, password)
    API->>AuthService: Login(email, password)
    AuthService->>UserRepo: FindUserByEmail(email)
    UserRepo-->>AuthService: (user)
    AuthService->>AuthService: Compare password hash
    AuthService->>AuthService: Generate JWT tokens
    AuthService-->>API: (tokens, userDetails)
    API-->>User: 200 OK (tokens, userDetails)
```

## 2. Friendship Management

### 2.1. Send Friend Request

*   **As a** user
*   **I want to** send a friend request to another user
*   **So that** we can become friends and send messages to each other.

**Scenario:**
*   User A sends a request to become friends with User B.
*   The system checks if they are already friends or if a request is already pending.
*   If not, it creates a new friendship record with a "pending" status.

```mermaid
sequenceDiagram
    participant UserA
    participant API
    participant FriendshipService
    participant FriendshipRepo
    UserA->>API: POST /api/friendships/requests (receiver_id)
    API->>FriendshipService: SendRequest(requesterID, receiverID)
    FriendshipService->>FriendshipRepo: AreFriends(requesterID, receiverID)
    FriendshipRepo-->>FriendshipService: false
    FriendshipService->>FriendshipRepo: CreateRequest(requesterID, receiverID)
    FriendshipRepo-->>FriendshipService: (friendship)
    FriendshipService-->>API: (friendship)
    API-->>UserA: 201 Created (friendship)
```

### 2.2. Respond to Friend Request

*   **As a** user
*   **I want to** accept or reject a friend request
*   **So that** I can control who is in my friends list.

**Scenario:**
*   User B receives a friend request from User A.
*   User B can choose to accept or reject the request.
*   If accepted, the friendship status is updated to "accepted", and both users are added to each other's friend lists.
*   If rejected, the status is updated to "rejected".

```mermaid
sequenceDiagram
    participant UserB
    participant API
    participant FriendshipService
    participant FriendshipRepo
    participant UserRepo
    UserB->>API: POST /api/friendships/requests/:id/respond (accept: true)
    API->>FriendshipService: RespondToRequest(friendshipID, receiverID, accept)
    FriendshipService->>FriendshipRepo: GetFriendRequests(receiverID, "pending")
    FriendshipRepo-->>FriendshipService: (friendship)
    FriendshipService->>UserRepo: AddFriend(requesterID, receiverID)
    UserRepo-->>FriendshipService: (ok)
    FriendshipService->>UserRepo: AddFriend(receiverID, requesterID)
    UserRepo-->>FriendshipService: (ok)
    FriendshipService->>FriendshipRepo: UpdateStatus(friendshipID, "accepted")
    FriendshipRepo-->>FriendshipService: (ok)
    FriendshipService-->>API: (ok)
    API-->>UserB: 200 OK
```

## 3. Group Management

### 3.1. Create a Group

*   **As a** user
*   **I want to** create a group with other users
*   **So that** we can have a group conversation.

**Scenario:**
*   A user provides a group name and a list of member IDs.
*   The system creates a new group, setting the creator as the first admin.
*   The creator and all specified members are added to the group's member list.

```mermaid
sequenceDiagram
    participant User
    participant API
    participant GroupService
    participant GroupRepo
    User->>API: POST /api/groups (name, member_ids)
    API->>GroupService: CreateGroup(creatorID, name, memberIDs)
    GroupService->>GroupRepo: CreateGroup(group)
    GroupRepo-->>GroupService: (createdGroup)
    GroupService-->>API: (createdGroup)
    API-->>User: 201 Created (createdGroup)
```

## 4. Messaging

### 4.1. Send a Direct Message

*   **As a** user
*   **I want to** send a message to a friend
*   **So that** we can communicate privately.

**Scenario:**
*   User A sends a message to User B, who is on their friend list.
*   The system verifies that they are friends.
*   The message is saved to the database.
*   The message is published to a Kafka topic for real-time delivery.

```mermaid
sequenceDiagram
    participant UserA
    participant API
    participant MessageService
    participant FriendshipRepo
    participant MessageRepo
    participant KafkaProducer
    UserA->>API: POST /api/messages (receiver_id, content)
    API->>MessageService: SendMessage(senderID, req)
    MessageService->>FriendshipRepo: AreFriends(senderID, receiverID)
    FriendshipRepo-->>MessageService: true
    MessageService->>MessageRepo: CreateMessage(message)
    MessageRepo-->>MessageService: (createdMessage)
    MessageService->>KafkaProducer: ProduceMessage(createdMessage)
    KafkaProducer-->>MessageService: (ok)
    MessageService-->>API: (createdMessage)
    API-->>UserA: 201 Created (createdMessage)
```

### 4.2. Send a Group Message

*   **As a** user
*   **I want to** send a message to a group
*   **So that** all members of the group can see it.

**Scenario:**
*   A user sends a message to a group they are a member of.
*   The system verifies that the sender is a member of the group.
*   The message is saved to the database.
*   The message is published to a Kafka topic, which will be consumed and distributed to all group members via WebSocket.

```mermaid
sequenceDiagram
    participant User
    participant API
    participant MessageService
    participant GroupRepo
    participant MessageRepo
    participant KafkaProducer
    User->>API: POST /api/messages (group_id, content)
    API->>MessageService: SendMessage(senderID, req)
    MessageService->>GroupRepo: GetGroup(groupID)
    GroupRepo-->>MessageService: (group with member list)
    MessageService->>MessageService: Verify sender is a member
    MessageService->>MessageRepo: CreateMessage(message)
    MessageRepo-->>MessageService: (createdMessage)
    MessageService->>KafkaProducer: ProduceMessage(createdMessage)
    KafkaProducer-->>MessageService: (ok)
    MessageService-->>API: (createdMessage)
    API-->>User: 201 Created (createdMessage)
```