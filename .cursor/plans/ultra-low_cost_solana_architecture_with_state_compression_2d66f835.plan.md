---
name: Ultra-Low Cost Solana Architecture with State Compression
overview: Optimize costs using Solana state compression, instruction data storage, event logs, and batch processing. Reduce account creation costs by 99%+ and transaction fees through batching and optimization.
todos:
  - id: design_instruction_data
    content: Design instruction data structures for predictions (no account creation)
    status: pending
  - id: implement_program_logs
    content: Implement program log emission for indexer to capture prediction data
    status: pending
    dependencies:
      - design_instruction_data
  - id: implement_batch_predictions
    content: Implement batch prediction creation (20 predictions per transaction)
    status: pending
    dependencies:
      - design_instruction_data
  - id: setup_compressed_nfts
    content: Set up Compressed NFTs (cNFTs) for critical predictions using Metaplex
    status: pending
  - id: implement_hybrid_storage
    content: Implement hybrid storage (cNFTs for critical, logs for regular)
    status: pending
    dependencies:
      - setup_compressed_nfts
      - implement_program_logs
  - id: optimize_account_creation
    content: Minimize account creation to only essential accounts (events, vaults)
    status: pending
  - id: implement_merkle_winner
    content: Implement off-chain winner calculation with Merkle tree (indexer)
    status: pending
    dependencies:
      - implement_program_logs
  - id: build_indexer_log_parser
    content: Build indexer to parse program logs and extract prediction data
    status: pending
    dependencies:
      - implement_program_logs
  - id: optimize_transaction_fees
    content: Implement priority fee optimization and batch processing
    status: pending
    dependencies:
      - implement_batch_predictions
  - id: test_cost_optimization
    content: Test and measure actual costs with 1M predictions simulation
    status: pending
    dependencies:
      - implement_hybrid_storage
      - optimize_transaction_fees
---

# Ultra-Low Cost Solana Architecture (100M Predictions)

## Cost Optimization Strategy

### Current Cost Breakdown (Without Optimization)

- 1M active predictions × 0.00089 SOL = **890 SOL** (account rent)
- 1M transactions × 0.000005 SOL = **5 SOL** (transaction fees)
- **Total: ~895 SOL**

### Optimized Cost Target

- Using state compression: **~1-10 SOL** (99% reduction)
- Using batch transactions: **~0.5-2 SOL** (60-90% reduction)
- **Total: ~1.5-12 SOL** (vs 895 SOL)

## State Compression Architecture

### What is State Compression?

- Uses Merkle trees to compress account data
- Stores only Merkle roots on-chain
- Full data stored in off-chain indexer
- **Cost reduction: 99%+** (e.g., $20M → $300 for 52M accounts)

### Implementation Options

#### Option 1: Compressed NFTs (cNFTs) - RECOMMENDED

- Use Metaplex Compressed NFTs for predictions
- Each prediction = 1 cNFT
- **Cost: ~0.000001 SOL per prediction** (vs 0.00089 SOL)
- **Reduction: 99.9%**

#### Option 2: State Compression Program

- Use Solana's state compression program directly
- Custom Merkle tree implementation
- More control, more complex

#### Option 3: Hybrid - Instruction Data + Logs

- Store minimal data in instruction data
- Use program logs for events
- Only create accounts for prize pools

## Recommended Architecture: Compressed NFTs + Minimal Accounts

### Account Structure (Minimal)

#### 1. Program State (1 account total)

```rust
pub struct ProgramState {
    pub admin: Pubkey,                    // 32 bytes
    pub fee_percentage: u8,               // 1 byte
    pub total_events: u64,                // 8 bytes
    pub event_counter: u64,               // 8 bytes
    // Total: ~49 bytes
}
```

#### 2. Event Account (Active events only, ~100)

```rust
pub struct Event {
    pub id: u64,                          // 8 bytes
    pub status: u8,                       // 1 byte
    pub prize_pool: u64,                  // 8 bytes
    pub merkle_root: [u8; 32],            // 32 bytes - root of all predictions
    pub end_time: i64,                    // 8 bytes
    // Total: ~57 bytes
}
// Cost: 100 events × 0.00089 SOL = 0.089 SOL
```

#### 3. Prize Pool Vault (Active events only, ~100)

```rust
pub struct PrizePool {
    pub event_id: Pubkey,                 // 32 bytes
    pub total: u64,                       // 8 bytes
    // Total: ~40 bytes
}
// Cost: 100 vaults × 0.00089 SOL = 0.089 SOL
```

#### 4. Game Outcomes (Store in instruction data, not accounts)

- Store game outcomes in Event account or instruction data
- No separate Game accounts needed
- Use Merkle tree for game data if needed

### Prediction Storage: Compressed NFTs

**Instead of Prediction Accounts, use Compressed NFTs:**

```rust
// Each prediction = 1 Compressed NFT
pub struct PredictionMetadata {
    pub user: Pubkey,                     // Stored in cNFT
    pub event_id: u64,                    // Stored in cNFT
    pub prediction_hash: [u8; 32],        // Stored in cNFT
    pub cost: u64,                        // Stored in cNFT
    pub timestamp: i64,                    // Stored in cNFT
    pub games: Vec<GamePrediction>,      // Stored off-chain, hash in cNFT
}
```

**Cost per prediction:**

- Compressed NFT: **~0.000001 SOL** (1,000x cheaper)
- 1M predictions: **1 SOL** (vs 890 SOL)
- **Savings: 889 SOL (99.9%)**

### Alternative: Instruction Data Storage

For even lower costs, store prediction data in instruction data:

```rust
// Store prediction in instruction data (no account needed)
pub struct CreatePrediction {
    pub user: Pubkey,                     // In instruction
    pub event_id: u64,                    // In instruction
    pub prediction_hash: [u8; 32],        // In instruction
    pub cost: u64,                        // In instruction
    // Emit as program log for indexer
}
```

**Cost:**

- Instruction data: **~0.000005 SOL** (transaction fee only)
- No account creation cost
- **Total for 1M predictions: 5 SOL** (transaction fees only)

## Ultra-Optimized Architecture

### Approach: Instruction Data + Program Logs + Minimal Accounts

#### On-Chain Storage (Minimal)

1. **Program State** (1 account) - Admin, config
2. **Event Accounts** (~100 active) - Event status, prize pool, Merkle root
3. **Prize Pool Vaults** (~100 active) - SOL storage
4. **User Stats** (optional, lazy creation) - Only when needed

#### Off-Chain Storage (Indexing Service)

1. **All prediction data** - From program logs
2. **Game data** - Team names, outcomes
3. **Event metadata** - Titles, descriptions
4. **Winner calculations** - Merkle trees

#### Prediction Flow (Cost-Optimized)

1. **User Creates Prediction**
   ```
   Instruction: create_prediction
                           - Data in instruction: user, event_id, prediction_hash, cost
                           - Emit program log with full prediction data
                           - Transfer SOL to prize pool vault
                           - No account creation!
                           - Cost: 0.000005 SOL (transaction fee only)
   ```

2. **Indexer Captures Data**

                                                - Monitor program logs
                                                - Extract prediction data
                                                - Store in database
                                                - Index for queries

3. **Event Completion**
   ```
   Instruction: complete_event
                           - Set game outcomes (in Event account or instruction data)
                           - Indexer calculates winners
                           - Generate Merkle tree
                           - Store Merkle root in Event account
                           - Cost: 0.000005 SOL
   ```

4. **Prize Claiming**
   ```
   Instruction: claim_prize
                           - Verify Merkle proof
                           - Transfer from vault
                           - Cost: 0.000005 SOL
   ```


### Cost Breakdown (Optimized)

**Account Creation:**

- Program state: 0.00089 SOL (one-time)
- 100 events: 0.089 SOL
- 100 prize pools: 0.089 SOL
- **Total accounts: ~0.18 SOL**

**Transaction Costs:**

- 1M predictions × 0.000005 SOL = **5 SOL**
- Event management: 0.01 SOL
- Prize claims: 0.5 SOL (100K winners)
- **Total transactions: ~5.5 SOL**

**Total On-Chain Cost: ~5.7 SOL** (vs 895 SOL)

**Reduction: 99.4%**

## Further Cost Optimizations

### 1. Batch Predictions

```rust
// Batch multiple predictions in one transaction
pub struct BatchCreatePrediction {
    pub predictions: Vec<PredictionData>, // Up to 10-20 per transaction
}
```

**Savings:** Reduce transaction count by 10-20x

**New cost:** 1M predictions / 20 = 50K transactions × 0.000005 = **0.25 SOL**

### 2. Use Priority Fees Strategically

- Use lower priority fees during low congestion
- Batch during low-fee periods
- **Savings: 50-80% on transaction fees**

### 3. Lazy Account Creation

- Only create UserStats when user claims prize
- Don't pre-create any accounts
- **Savings: Eliminate unnecessary accounts**

### 4. Account Reuse

- Reuse Event accounts for similar events
- Update instead of create new
- **Savings: Reduce account creation**

### 5. Instruction Data Optimization

- Pack data efficiently (u8, u16 instead of u64 where possible)
- Use enums instead of strings
- **Savings: Reduce transaction size, lower fees**

## Final Optimized Cost Estimate

### Scenario 1: Instruction Data + Batching

- Account creation: **0.18 SOL**
- Transactions (batched): **0.25 SOL**
- **Total: ~0.43 SOL** for 1M active predictions

### Scenario 2: Compressed NFTs

- Account creation: **0.18 SOL**
- Compressed NFTs: **1 SOL** (1M predictions)
- Transactions: **0.5 SOL**
- **Total: ~1.68 SOL** for 1M active predictions

### Scenario 3: Hybrid (Best Balance)

- Account creation: **0.18 SOL**
- Critical predictions as cNFTs: **0.1 SOL** (100K critical)
- Regular predictions in logs: **0.25 SOL** (900K in batches)
- Transactions: **0.5 SOL**
- **Total: ~1.03 SOL** for 1M active predictions

## Program Instructions (Optimized)

### Core Instructions

1. **`initialize`** - Initialize program (one-time)

                                                - Cost: 0.00089 SOL

2. **`create_event`** - Create event (admin)

                                                - Creates Event account
                                                - Creates Prize Pool vault
                                                - Cost: 0.00178 SOL per event

3. **`create_prediction`** - Create prediction (user)

                                                - Data in instruction
                                                - Emit program log
                                                - Transfer SOL to vault
                                                - Cost: 0.000005 SOL (no account)

4. **`batch_create_predictions`** - Batch predictions (user)

                                                - Up to 20 predictions per transaction
                                                - Cost: 0.000005 SOL (for 20 predictions)

5. **`complete_event`** - Complete event (admin)

                                                - Set outcomes
                                                - Update Merkle root
                                                - Cost: 0.000005 SOL

6. **`claim_prize`** - Claim prize (user)

                                                - Verify Merkle proof
                                                - Transfer prize
                                                - Cost: 0.000005 SOL

## Indexing Service (Critical)

### Responsibilities

1. **Monitor Program Logs**

                                                - Capture all prediction data from logs
                                                - Store in database
                                                - Index for queries

2. **Calculate Winners**

                                                - When event completes
                                                - Generate Merkle tree
                                                - Provide Merkle proofs

3. **Query API**

                                                - User predictions
                                                - Event predictions
                                                - Search and filter
                                                - Real-time updates

### Cost: ~$50-500/month (scales with usage)

## Comparison: Approaches

| Approach | Account Cost | Transaction Cost | Total | Reduction |

|----------|-------------|------------------|-------|-----------|

| **Original (Full On-Chain)** | 890 SOL | 5 SOL | 895 SOL | - |

| **Active Events Only** | 891 SOL | 5 SOL | 896 SOL | 0% |

| **Instruction Data** | 0.18 SOL | 5 SOL | 5.18 SOL | 99.4% |

| **Instruction Data + Batching** | 0.18 SOL | 0.25 SOL | 0.43 SOL | 99.95% |

| **Compressed NFTs** | 0.18 SOL | 1.5 SOL | 1.68 SOL | 99.8% |

| **Hybrid (Recommended)** | 0.18 SOL | 1.03 SOL | 1.21 SOL | 99.86% |

## Recommended Approach: Hybrid

### Why Hybrid?

1. **Lowest cost** (1.21 SOL vs 895 SOL)
2. **Verifiable** (predictions in logs/cNFTs)
3. **Queryable** (indexing service)
4. **Scalable** (handles 100M+ predictions)

### Implementation

- **Critical predictions**: Compressed NFTs (for important events)
- **Regular predictions**: Instruction data + program logs (for cost)
- **Events**: Minimal accounts (active only)
- **Prizes**: Vault accounts (necessary for SOL storage)

## Migration Path

### Phase 1: Instruction Data Approach

- Start with instruction data + logs
- Lowest cost, easiest to implement
- Cost: ~0.43 SOL for 1M predictions

### Phase 2: Add Compressed NFTs

- Add cNFTs for critical predictions
- Better verifiability
- Cost: ~1.21 SOL for 1M predictions

### Phase 3: Optimize Further

- Batch operations
- Priority fee optimization
- Account reuse
- Cost: <1 SOL for 1M predictions

## Conclusion

**Best Approach: Hybrid (Instruction Data + Compressed NFTs)**

- **Cost: ~1.21 SOL** for 1M active predictions
- **Reduction: 99.86%** vs full on-chain
- **Scalable: 100M+ predictions** (only active on-chain)
- **Verifiable: Predictions verifiable via logs/cNFTs**
- **Queryable: Full data in indexing service**

**Key Optimizations:**

1. ✅ Instruction data instead of accounts
2. ✅ Batch transactions (20x reduction)
3. ✅ Compressed NFTs for critical data
4. ✅ Active events only
5. ✅ Program logs for indexer
6. ✅ Minimal account creation

This approach reduces costs by **99.86%** while maintaining security and verifiability.