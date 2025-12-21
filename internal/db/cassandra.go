package db

import (
	"log"
	"time"

	"github.com/gocql/gocql"
)

type CassandraClient struct {
	Session *gocql.Session
}

func NewCassandraClient(hosts []string, keyspace, username, password string) (*CassandraClient, error) {
	var session *gocql.Session
	var err error

	// Retry loop for startup resilience (wait for DB to be ready)
	for i := 0; i < 20; i++ {
		cluster := gocql.NewCluster(hosts...)
		cluster.Keyspace = keyspace // Initial attempt with keyspace
		cluster.Consistency = gocql.Quorum
		cluster.ProtoVersion = 4
		cluster.ConnectTimeout = 10 * time.Second
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: username,
			Password: password,
		}
		cluster.RetryPolicy = &gocql.SimpleRetryPolicy{NumRetries: 3}

		session, err = cluster.CreateSession()
		if err == nil {
			log.Println("Successfully connected to Cassandra.")
			break
		}

		// Connection failed.
		// Check if it's because keyspace doesn't exist, or network/startup issue.
		// We try to connect without keyspace to distinguish.
		cluster.Keyspace = ""
		sysSession, sysErr := cluster.CreateSession()
		if sysErr == nil {
			// Connected to system! Keyspace probably doesn't exist.
			log.Printf("Connected to Cassandra system. Creating keyspace '%s'...", keyspace)
			if err := createKeyspace(sysSession, keyspace); err != nil {
				log.Printf("Failed to create keyspace: %v", err)
			}
			sysSession.Close()
			// Loop continues -> Next iteration will try to connect with keyspace again.
		} else {
			// Still failed. Likely network/startup issue.
			log.Printf("Failed to connect to Cassandra (attempt %d/20): %v", i+1, err)
		}

		time.Sleep(3 * time.Second)
	}

	if err != nil {
		return nil, err // All retries failed
	}

	err = createTables(session)
	if err != nil {
		log.Printf("Error creating tables: %v", err)
		session.Close()
		return nil, err
	}

	return &CassandraClient{Session: session}, nil
}

func createKeyspace(session *gocql.Session, keyspace string) error {
	query := `CREATE KEYSPACE IF NOT EXISTS ` + keyspace + ` WITH replication = {
		'class': 'SimpleStrategy',
		'replication_factor': 1
	};`
	return session.Query(query).Exec()
}

func createTables(session *gocql.Session) error {
	// Table 1: Messages by Conversation (Chat History)
	// Partition: conversation_id (Grouping messages together)
	// Cluster: message_id (TimeUUID implies timestamp, so it orders by time naturally)
	msgsQuery := `CREATE TABLE IF NOT EXISTS messages (
		conversation_id text,
		message_id timeuuid,
		sender_id text,
		receiver_id text,
		group_id text,
		content text,
		content_type text,
		media_urls list<text>,
		is_read boolean,
		seen_by set<text>,
		delivered_to set<text>,
		reactions text, -- JSON stored as text
		reply_to_id text,
		is_marketplace boolean,
		product_id text,
		created_at timestamp,
		updated_at timestamp,
		is_deleted boolean,
		PRIMARY KEY ((conversation_id), message_id)
	) WITH CLUSTERING ORDER BY (message_id DESC);`
	if err := session.Query(msgsQuery).Exec(); err != nil {
		return err
	}

	// Table 1b: Message Metadata (Mutable fields - stays forever for archived messages)
	// Partition: conversation_id (same as messages)
	// Used for: reactions, seen_by, delivered_to, is_deleted, is_edited
	metadataQuery := `CREATE TABLE IF NOT EXISTS message_metadata (
		conversation_id text,
		message_id timeuuid,
		reactions text,
		seen_by set<text>,
		delivered_to set<text>,
		is_deleted boolean,
		is_edited boolean,
		PRIMARY KEY ((conversation_id), message_id)
	) WITH CLUSTERING ORDER BY (message_id DESC);`
	if err := session.Query(metadataQuery).Exec(); err != nil {
		return err
	}

	// Table 1c: Archive Index (Pointers to MinIO cold storage)
	// Partition: conversation_id
	// Cluster: month (YYYY-MM format for range queries)
	archiveIndexQuery := `CREATE TABLE IF NOT EXISTS messages_archive_index (
		conversation_id text,
		month text,
		archive_path text,
		message_count int,
		archived_at timestamp,
		PRIMARY KEY ((conversation_id), month)
	) WITH CLUSTERING ORDER BY (month DESC);`
	if err := session.Query(archiveIndexQuery).Exec(); err != nil {
		return err
	}

	// Table 2: User Inbox (Recent Conversations)
	// Partition: user_id
	// Proper design: ONE row per conversation, last_message_at is a regular column
	inboxQuery := `CREATE TABLE IF NOT EXISTS user_inbox (
		user_id text,
		conversation_id text,
		conversation_name text,
		conversation_avatar text,
		is_group boolean,
		is_marketplace boolean,
		last_message_content text,
		last_message_sender_id text,
		last_message_sender_name text,
		last_message_at timestamp,
		PRIMARY KEY ((user_id), is_marketplace, conversation_id)
	) WITH CLUSTERING ORDER BY (is_marketplace ASC, conversation_id ASC);`
	if err := session.Query(inboxQuery).Exec(); err != nil {
		return err
	}

	// Table 3: Unread Counts (Counter Table)
	counterQuery := `CREATE TABLE IF NOT EXISTS conversation_unread (
		user_id text,
		conversation_id text,
		unread_count counter,
		PRIMARY KEY ((user_id), conversation_id)
	);`
	if err := session.Query(counterQuery).Exec(); err != nil {
		return err
	}

	// Table 4: Group Activities (System Messages)
	// Partition: group_id
	// Cluster: created_at DESC, activity_id DESC
	activitiesQuery := `CREATE TABLE IF NOT EXISTS group_activities (
		group_id text,
		activity_id timeuuid,
		activity_type text,
		actor_id text,
		actor_name text,
		target_id text,
		target_name text,
		metadata text,
		created_at timestamp,
		PRIMARY KEY ((group_id), created_at, activity_id)
	) WITH CLUSTERING ORDER BY (created_at DESC, activity_id DESC);`
	if err := session.Query(activitiesQuery).Exec(); err != nil {
		return err
	}

	return nil
}

func (c *CassandraClient) Close() {
	if c.Session != nil {
		c.Session.Close()
	}
}
