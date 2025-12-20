#!/bin/bash

# Configuration
USERS=100000
MESSAGES_PER_USER=5
ENDPOINT="http://localhost:8080/api/messages"
WS_ENDPOINT="ws://localhost:8081/ws"
WS_PROTOCOL="connectify.auth"

# Get JWT token
TOKEN=$(curl -s -X POST -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password"}' \
  $ENDPOINT/auth/login | jq -r '.access_token')

if [ -z "$TOKEN" ]; then
  echo "Failed to get JWT token"
  exit 1
fi

# Run HTTP load test
echo "Starting HTTP load test..."
ab -n $((USERS*MESSAGES_PER_USER)) -c 1000 -H "Authorization: Bearer $TOKEN" \
  -T "application/json" -p test_message.json $ENDPOINT/messages

# Run WebSocket test
echo "Starting WebSocket connections..."
for i in {1..1000}; do
  wscat -c "$WS_ENDPOINT" -s "$WS_PROTOCOL" -s "$TOKEN" > /dev/null 2>&1 &
done

echo "Load testing in progress..."
wait
echo "Load test completed"
