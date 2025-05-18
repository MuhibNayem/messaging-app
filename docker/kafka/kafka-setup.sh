#!/bin/sh

KAFKA_BROKER=$1
TOPIC=$2

echo "Waiting for Kafka to be ready..."
until kafka-topics --bootstrap-server $KAFKA_BROKER --list; do
  sleep 1
done

echo "Creating topic $TOPIC with 10 partitions and replication factor 2"
kafka-topics --bootstrap-server $KAFKA_BROKER \
  --create \
  --topic $TOPIC \
  --partitions 10 \
  --replication-factor 2 \
  --config min.insync.replicas=2

echo "Topic $TOPIC created successfully"