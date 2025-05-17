#!/bin/bash

# Health check with replica set recovery
MAX_ATTEMPTS=5
ATTEMPT=0
SLEEP_SECONDS=5

MONGO_URI="mongodb://root:example@localhost:27017/admin"
SUCCESS=false

while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
  # Check replica set status
  RS_STATUS=$(mongosh "$MONGO_URI" --quiet --eval "
    try {
      const status = rs.status();
      // Check if we have at least one PRIMARY and one SECONDARY
      const hasPrimary = status.members.some(m => m.state === 1);
      const hasSecondary = status.members.some(m => m.state === 2);
      
      if (!hasPrimary || !hasSecondary) {
        // Attempt reconfiguration if members exist but roles are wrong
        if (status.members.length >= 2) {
          print('attempting_reconfig');
          rs.reconfig(${RS_CONFIG}, {force: true});
          // Wait for reconfiguration to take effect
          sleep(5000);
          // Check status again after reconfig
          const newStatus = rs.status();
          if (newStatus.members.some(m => m.state === 1) && newStatus.members.some(m => m.state === 2)) {
            print('healthy_after_reconfig');
            quit(0);
          }
        } else {
          // If members are missing, try full reinitialization
          print('attempting_reinit');
          rs.initiate(${RS_CONFIG});
          sleep(10000); // Wait longer for initial election
          const newStatus = rs.status();
          if (newStatus.members.some(m => m.state === 1)) {
            print('healthy_after_reinit');
            quit(0);
          }
        }
      } else {
        print('healthy');
        quit(0);
      }
    } catch (e) {
      if (e.codeName === 'NotYetInitialized') {
        print('not_initialized');
        rs.initiate(${RS_CONFIG});
        sleep(10000);
        const newStatus = rs.status();
        if (newStatus.members.some(m => m.state === 1)) {
          print('healthy_after_init');
          quit(0);
        }
      } else {
        print('error_' + e.codeName);
      }
      quit(1);
    }
  ")

  case "$RS_STATUS" in
    *healthy*)
      SUCCESS=true
      break
      ;;
    *attempting_reconfig*|*attempting_reinit*|*not_initialized*)
      echo "Replica set recovery in progress: $RS_STATUS"
      ;;
    *)
      echo "Replica set check failed: $RS_STATUS"
      ;;
  esac

  ATTEMPT=$((ATTEMPT+1))
  sleep $SLEEP_SECONDS
done

if [ "$SUCCESS" = true ]; then
  exit 0
else
  exit 1
fi