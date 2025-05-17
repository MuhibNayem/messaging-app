print("Starting replica set initialization...");
const rsConfig = {
  _id: "rs0",
  members: [
    { _id: 0, host: "mongodb1:27017", priority: 2 },
    { _id: 1, host: "mongodb2:27017", priority: 1 },
    { _id: 2, host: "mongodb3:27017", priority: 1 },
  ],
};

try {
  const status = rs.status();
  if (status.ok) {
    print("Replica set status:", JSON.stringify(status));

    // Check if all members are included
    if (status.members.length < 3) {
      print("Reconfiguring replica set to include all members...");
      rs.reconfig(rsConfig, { force: true });
    } else {
      print("Replica set already contains all members. No changes needed.");
    }
  }
} catch (e) {
  print("Error checking replica set status:", e);
  print("Initiating replica set with configuration:", JSON.stringify(rsConfig));

  const result = rs.initiate(rsConfig);
  print("Initiation result:", JSON.stringify(result));
}

sleep(5000);

try {
  const finalStatus = rs.status();
  print("Final replica set status:", JSON.stringify(finalStatus));
} catch (e) {
  print("Error getting final status:", e);
}
