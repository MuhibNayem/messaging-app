// /docker-entrypoint-initdb.d/init_mongo.js

const rsConfig = {
  _id: "rs0",
  members: [
    { _id: 0, host: "mongodb1:27017" },
    { _id: 1, host: "mongodb2:27017" },
  ],
};

try {
  const status = rs.status();
  if (!status.ok || status.members.length < 2) {
    print("Replica set not initialized or partially configured. Initiating...");
    rs.initiate(rsConfig);
  } else {
    print("Replica set already initialized. Skipping...");
  }
} catch (e) {
  if (e.codeName === "NotYetInitialized") {
    print("Replica set not yet initialized. Initiating...");
    rs.initiate(rsConfig);
  } else {
    print("Error checking replica set status: " + e);
  }
}
