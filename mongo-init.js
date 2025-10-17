// MongoDB initialization script
// This script runs when the MongoDB container starts for the first time

// Switch to the sol_db database
db = db.getSiblingDB('sol_db');

// Create indexes for better performance
db.users.createIndex({ "email": 1 }, { unique: true });
db.users.createIndex({ "walletAddress": 1 }, { unique: true });

print("Database 'sol_db' initialized");
print("Users collection created - users will be added only through authentication");

// Switch to the admin_db database
db = db.getSiblingDB('admin_db');

// Create indexes for admin collection
db.admins.createIndex({ "walletAddress": 1 }, { unique: true });
db.admins.createIndex({ "role": 1 });

// Insert the admin wallet address
db.admins.insertOne({
  walletAddress: "4tyf8awFN919MwuhDs7AkPN4gY6RJNawSwqisK3dcHdy",
  role: "admin",
  createdAt: new Date(),
  isActive: true
});

print("Database 'admin_db' initialized");
print("Admin collection created with initial admin wallet: 4tyf8awFN919MwuhDs7AkPN4gY6RJNawSwqisK3dcHdy");
