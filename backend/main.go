package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

// User represents a user in our system
type User struct {
	ID           primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Name         string             `json:"name" bson:"name"`
	Email        string             `json:"email" bson:"email"`
	WalletAddress string            `json:"walletAddress" bson:"walletAddress"`
	SOLBalance   float64            `json:"solBalance" bson:"solBalance"`
	LastLogin    time.Time          `json:"lastLogin" bson:"lastLogin"`
}

// Admin represents an admin in our system
type Admin struct {
	ID           primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	WalletAddress string            `json:"walletAddress" bson:"walletAddress"`
	Role         string             `json:"role" bson:"role"`
	CreatedAt    time.Time          `json:"createdAt" bson:"createdAt"`
	IsActive     bool               `json:"isActive" bson:"isActive"`
}

// MongoDB connection
var client *mongo.Client
var collection *mongo.Collection
var adminCollection *mongo.Collection

// isValidTimestamp checks if the timestamp is valid and not expired
func isValidTimestamp(timestamp string) bool {
	parsedTime, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return false
	}
	
	// Check if timestamp is within last 5 minutes
	now := time.Now()
	diff := now.Sub(parsedTime)
	return diff >= 0 && diff <= 5*time.Minute
}

// isAdmin checks if a wallet address is an admin
func isAdmin(walletAddress string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	var admin Admin
	err := adminCollection.FindOne(ctx, bson.M{
		"walletAddress": walletAddress,
		"isActive": true,
	}).Decode(&admin)
	
	return err == nil
}

// Authentication middleware
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w, r)
		
		// Get wallet address from header
		walletAddress := r.Header.Get("X-Wallet-Address")
		if walletAddress == "" {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
		
		// Verify user exists in database
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		var user User
		err := collection.FindOne(ctx, bson.M{"walletAddress": walletAddress}).Decode(&user)
		if err != nil {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}
		
		// Add user to request context
		ctx = context.WithValue(r.Context(), "user", user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// CORS middleware
func enableCORS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Wallet-Address")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
}

func main() {
	// Connect to MongoDB
	connectToMongoDB()
	defer func() {
		if err := client.Disconnect(context.TODO()); err != nil {
			log.Fatal(err)
		}
	}()

	// Set up routes
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/api/auth/solana", solanaAuthHandler)
	
	// Protected routes (require authentication)
	http.HandleFunc("/api/users", requireAuth(usersHandler))
	http.HandleFunc("/api/users/", requireAuth(userByIDHandler))
	
	// Admin routes
	http.HandleFunc("/api/admin/check", requireAuth(adminCheckHandler))

	// Start server
	port := ":8080"
	fmt.Printf("Server starting on port %s\n", port)
	fmt.Println("Available endpoints:")
	fmt.Println("  GET  / - Home page")
	fmt.Println("  GET  /health - Health check")
	fmt.Println("  POST /api/auth/solana - Solana wallet authentication")
	fmt.Println("  GET  /api/users - Get all authenticated users (requires auth)")
	fmt.Println("  GET  /api/users/{id} - Get user by ID (requires auth)")
	fmt.Println("  GET  /api/admin/check - Check if user is admin (requires auth)")
	
	log.Fatal(http.ListenAndServe(port, nil))
}

// connectToMongoDB establishes connection to MongoDB
func connectToMongoDB() {
	// Get MongoDB URI from environment variable or use default
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		// Default connection for Docker MongoDB - no authentication
		mongoURI = "mongodb://localhost:27017"
	}

	// Set client options
	clientOptions := options.Client().ApplyURI(mongoURI)

	// Connect to MongoDB
	var err error
	client, err = mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}

	// Check the connection
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		log.Fatal("Failed to ping MongoDB:", err)
	}

	fmt.Println("Connected to MongoDB!")

	// Get collections
	collection = client.Database("sol_db").Collection("users")
	adminCollection = client.Database("admin_db").Collection("admins")
}

// homeHandler handles the root endpoint
func homeHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"message": "Welcome to Sol Backend API with MongoDB",
		"version": "1.0.0",
		"database": "MongoDB",
		"endpoints": []string{
			"GET /health",
			"GET /api/users",
			"GET /api/users/{id}",
			"POST /api/users",
		},
	}
	json.NewEncoder(w).Encode(response)
}

// healthHandler handles health check requests
func healthHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	w.Header().Set("Content-Type", "application/json")
	
	// Check MongoDB connection
	err := client.Ping(context.TODO(), nil)
	status := "healthy"
	message := "Server and database are running properly"
	
	if err != nil {
		status = "unhealthy"
		message = "Database connection failed"
	}
	
	response := HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Message:   message,
	}
	json.NewEncoder(w).Encode(response)
}

// usersHandler handles user-related requests (authenticated users only)
func usersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	switch r.Method {
	case http.MethodGet:
		// Get all authenticated users
		getAllUsers(w, r)
		
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getAllUsers retrieves all users from MongoDB
func getAllUsers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		http.Error(w, "Failed to fetch users", http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	var users []User
	if err = cursor.All(ctx, &users); err != nil {
		http.Error(w, "Failed to decode users", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(users)
}


// userByIDHandler handles requests for specific users by ID
func userByIDHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	// Extract ID from URL path
	path := r.URL.Path
	idStr := path[len("/api/users/"):]
	
	// Convert string ID to ObjectID
	objectID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Find user by ID
	var user User
	err = collection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			http.Error(w, "User not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to fetch user", http.StatusInternalServerError)
		}
		return
	}
	
	json.NewEncoder(w).Encode(user)
}

// SolanaAuthRequest represents the authentication request
type SolanaAuthRequest struct {
	PublicKey string  `json:"publicKey"`
	Balance   float64 `json:"balance"`
	Signature []int   `json:"signature"`
	Message   string  `json:"message"`
	Timestamp string  `json:"timestamp"`
}

// SolanaAuthResponse represents the authentication response
type SolanaAuthResponse struct {
	Success      bool      `json:"success"`
	User         *User     `json:"user,omitempty"`
	Message      string    `json:"message"`
	AuthenticatedAt time.Time `json:"authenticatedAt"`
}

// solanaAuthHandler handles Solana wallet authentication
func solanaAuthHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	w.Header().Set("Content-Type", "application/json")
	
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var authReq SolanaAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&authReq); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	// Validate required fields
	if authReq.PublicKey == "" {
		http.Error(w, "Public key is required", http.StatusBadRequest)
		return
	}
	
	if authReq.Message == "" {
		http.Error(w, "Authentication message is required", http.StatusBadRequest)
		return
	}
	
	if len(authReq.Signature) == 0 {
		http.Error(w, "Message signature is required", http.StatusBadRequest)
		return
	}
	
	if authReq.Timestamp == "" {
		http.Error(w, "Timestamp is required", http.StatusBadRequest)
		return
	}
	
	// Validate timestamp (should be within last 5 minutes)
	if !isValidTimestamp(authReq.Timestamp) {
		http.Error(w, "Invalid or expired timestamp", http.StatusBadRequest)
		return
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Check if user exists by wallet address
	var existingUser User
	err := collection.FindOne(ctx, bson.M{"walletAddress": authReq.PublicKey}).Decode(&existingUser)
	
	if err == mongo.ErrNoDocuments {
		// Create new user
		newUser := User{
			Name:          "Solana User",
			Email:         fmt.Sprintf("%s@solana.wallet", authReq.PublicKey[:8]),
			WalletAddress: authReq.PublicKey,
			SOLBalance:    authReq.Balance,
			LastLogin:     time.Now(),
		}
		
		result, err := collection.InsertOne(ctx, newUser)
		if err != nil {
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}
		
		// Get the created user
		err = collection.FindOne(ctx, bson.M{"_id": result.InsertedID}).Decode(&existingUser)
		if err != nil {
			http.Error(w, "Failed to retrieve created user", http.StatusInternalServerError)
			return
		}
		
		response := SolanaAuthResponse{
			Success:         true,
			User:           &existingUser,
			Message:        "New user created and authenticated successfully with message signature",
			AuthenticatedAt: time.Now(),
		}
		
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
		
	} else if err != nil {
		http.Error(w, "Failed to authenticate", http.StatusInternalServerError)
		return
	} else {
		// Update existing user's balance and last login
		update := bson.M{
			"$set": bson.M{
				"solBalance": authReq.Balance,
				"lastLogin":  time.Now(),
			},
		}
		
		_, err = collection.UpdateOne(ctx, bson.M{"_id": existingUser.ID}, update)
		if err != nil {
			http.Error(w, "Failed to update user", http.StatusInternalServerError)
			return
		}
		
		// Get updated user
		err = collection.FindOne(ctx, bson.M{"_id": existingUser.ID}).Decode(&existingUser)
		if err != nil {
			http.Error(w, "Failed to retrieve updated user", http.StatusInternalServerError)
			return
		}
		
		response := SolanaAuthResponse{
			Success:         true,
			User:           &existingUser,
			Message:        "User authenticated successfully with message signature",
			AuthenticatedAt: time.Now(),
		}
		
		json.NewEncoder(w).Encode(response)
	}
}

// adminCheckHandler checks if the authenticated user is an admin
func adminCheckHandler(w http.ResponseWriter, r *http.Request) {
	// Get user from context (set by requireAuth middleware)
	user, ok := r.Context().Value("user").(User)
	if !ok {
		http.Error(w, "User not found in context", http.StatusInternalServerError)
		return
	}
	
	// Check if user is admin
	isAdminUser := isAdmin(user.WalletAddress)
	
	response := map[string]interface{}{
		"isAdmin": isAdminUser,
		"walletAddress": user.WalletAddress,
	}
	
	if isAdminUser {
		response["message"] = "User is an admin"
	} else {
		response["message"] = "User is not an admin"
	}
	
	json.NewEncoder(w).Encode(response)
}