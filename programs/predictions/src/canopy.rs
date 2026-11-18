//! The stored upper part of an event's two slip trees.
//!
//! Both trees keep the depth of the slip tree. The bottom `levels` hashes of a proof come
//! from the client and are folded into a chunk node. A chunk is `2^levels` coupons. Every
//! node from the chunk node up to the root is stored here and written by the program.
//!
//! The account grows by one record per chunk, when the first coupon of that chunk is placed.
//! A record holds the claimed bits of its chunk and four hashes: for each tree, the chunk
//! node (slot A) and one node above it (slot B). Slot B of record `l` is the node at height
//! `r` and position `j` with `l + 1 == (2j + 1) << (r - 1)`, which is the last chunk of
//! that node's left half. So a node gets a slot when its right half can first hold a coupon.
//! Until then its right half is empty and the node is derived from its left child.
//! The root sits in the header.

use anchor_lang::solana_program::hash::hashv;

pub const TAG: [u8; 8] = *b"canopy01";
pub const HEADER: usize = 128;
const EVENT_AT: usize = 8;
const LEVELS_AT: usize = 40;
const BUMP_AT: usize = 41;
pub const COUNT_AT: usize = 44;
const REVEALS_AT: usize = 48;
const ROOT_AT: usize = 56;
/// Kept rows of every revealed coupon. The two roots end at this offset, and the
/// right edge of the entry tree starts at `HEADER`, so these eight bytes were spare.
pub const REVEALED_ROWS_AT: usize = 120;

/// Rows already published. Totals have to add up to this, so a list cannot invent
/// or drop revealed rows as a whole.
pub fn revealed_rows(data: &[u8]) -> u64 {
    if data.len() < REVEALED_ROWS_AT + 8 {
        return 0;
    }
    u64::from_le_bytes(data[REVEALED_ROWS_AT..REVEALED_ROWS_AT + 8].try_into().unwrap())
}

/// Adds the kept rows of one reveal. Returns false when the running sum would overflow.
pub fn add_revealed_rows(data: &mut [u8], rows: u64) -> bool {
    if data.len() < REVEALED_ROWS_AT + 8 {
        return false;
    }
    let Some(next) = revealed_rows(data).checked_add(rows) else {
        return false;
    };
    data[REVEALED_ROWS_AT..REVEALED_ROWS_AT + 8].copy_from_slice(&next.to_le_bytes());
    true
}

pub const ENTRY: usize = 0;
pub const REVEAL: usize = 1;

pub type Node = [u8; 32];

fn pair(left: &Node, right: &Node) -> Node {
    hashv(&[left, right]).to_bytes()
}

fn bitmap_len(levels: usize) -> usize {
    ((1usize << levels) + 7) / 8
}

/// Bytes one chunk adds to the account.
pub fn record_len(levels: usize) -> usize {
    bitmap_len(levels) + 4 * 32
}

/// Account size with `records` chunks allocated.
pub fn account_len(levels: usize, records: usize) -> usize {
    HEADER + 32 * levels + records * record_len(levels)
}

/// Folds a leaf up the client's bottom hashes. `proof[0]` is the leaf's sibling.
pub fn fold(index: u32, leaf: &Node, proof: &[Node]) -> Node {
    let mut node = *leaf;
    let mut cursor = index;
    for sibling in proof {
        node = if cursor & 1 == 0 { pair(&node, sibling) } else { pair(sibling, &node) };
        cursor >>= 1;
    }
    node
}

pub struct Canopy {
    levels: usize,
    upper: usize,
    records: usize,
    zeros: Vec<Node>,
}

impl Canopy {
    fn shaped(levels: usize, depth: usize, len: usize) -> Option<Self> {
        if levels == 0 || levels >= depth || depth > 31 {
            return None;
        }
        let fixed = account_len(levels, 0);
        let record = record_len(levels);
        if len < fixed || (len - fixed) % record != 0 {
            return None;
        }
        let records = (len - fixed) / record;
        if records > 1usize << (depth - levels) {
            return None;
        }
        let mut zeros = Vec::with_capacity(depth + 1);
        let mut node = crate::slip_zero(0);
        zeros.push(node);
        for _ in 0..depth {
            node = pair(&node, &node);
            zeros.push(node);
        }
        Some(Self { levels, upper: depth - levels, records, zeros })
    }

    /// Writes the header and the first records of a new, zeroed account.
    pub fn init(data: &mut [u8], event: &[u8; 32], levels: usize, bump: u8, depth: usize) -> Option<Self> {
        let canopy = Self::shaped(levels, depth, data.len())?;
        if data[..8] != [0u8; 8] {
            return None;
        }
        data[..8].copy_from_slice(&TAG);
        data[EVENT_AT..EVENT_AT + 32].copy_from_slice(event);
        data[LEVELS_AT] = levels as u8;
        data[BUMP_AT] = bump;
        for tree in [ENTRY, REVEAL] {
            canopy.set_root(data, tree, &canopy.zeros[depth]);
        }
        for record in 0..canopy.records {
            canopy.init_record(data, record);
        }
        Some(canopy)
    }

    pub fn open(data: &[u8], depth: usize) -> Option<Self> {
        if data.len() < HEADER || data[..8] != TAG {
            return None;
        }
        Self::shaped(data[LEVELS_AT] as usize, depth, data.len())
    }

    pub fn levels(&self) -> usize {
        self.levels
    }

    pub fn records(&self) -> usize {
        self.records
    }

    /// The empty leaf.
    pub fn empty(&self) -> Node {
        self.zeros[0]
    }

    pub fn event(data: &[u8]) -> [u8; 32] {
        data[EVENT_AT..EVENT_AT + 32].try_into().unwrap()
    }

    pub fn count(data: &[u8]) -> u32 {
        u32::from_le_bytes(data[COUNT_AT..COUNT_AT + 4].try_into().unwrap())
    }

    pub fn reveal_count(data: &[u8]) -> u32 {
        u32::from_le_bytes(data[REVEALS_AT..REVEALS_AT + 4].try_into().unwrap())
    }

    pub fn root(&self, data: &[u8], tree: usize) -> Node {
        let at = ROOT_AT + 32 * tree;
        data[at..at + 32].try_into().unwrap()
    }

    fn set_root(&self, data: &mut [u8], tree: usize, node: &Node) {
        let at = ROOT_AT + 32 * tree;
        data[at..at + 32].copy_from_slice(node);
    }

    fn record_at(&self, record: usize) -> usize {
        account_len(self.levels, record)
    }

    /// Slot 0 is the chunk node, slot 1 the node above it that this record carries.
    fn slot_at(&self, tree: usize, record: usize, slot: usize) -> usize {
        self.record_at(record) + bitmap_len(self.levels) + 32 * (2 * tree + slot)
    }

    fn slot(&self, data: &[u8], tree: usize, record: usize, slot: usize) -> Node {
        let at = self.slot_at(tree, record, slot);
        data[at..at + 32].try_into().unwrap()
    }

    fn set_slot(&self, data: &mut [u8], tree: usize, record: usize, slot: usize, node: &Node) {
        let at = self.slot_at(tree, record, slot);
        data[at..at + 32].copy_from_slice(node);
    }

    /// The record that stores the node at `height` (1 or more) and `position`.
    fn home(height: usize, position: usize) -> usize {
        ((2 * position + 1) << (height - 1)) - 1
    }

    /// The node `height` levels above the chunk nodes. Height 0 is a chunk node.
    fn node(&self, data: &[u8], tree: usize, height: usize, position: usize) -> Node {
        if height == self.upper {
            return self.root(data, tree);
        }
        if (position << height) >= self.records {
            return self.zeros[self.levels + height];
        }
        // Walk down the left edge to the highest stored node. Everything to its right
        // is past the last record, so it is empty.
        let mut at = height;
        let mut cursor = position;
        let mut node = loop {
            if at == 0 {
                break self.slot(data, tree, cursor, 0);
            }
            let home = Self::home(at, cursor);
            if home < self.records {
                break self.slot(data, tree, home, 1);
            }
            at -= 1;
            cursor <<= 1;
        };
        while at < height {
            node = pair(&node, &self.zeros[self.levels + at]);
            at += 1;
        }
        node
    }

    /// Fills a record that was just added to the account. Its chunk is empty in both trees.
    pub fn init_record(&self, data: &mut [u8], record: usize) {
        let start = self.record_at(record);
        data[start..start + record_len(self.levels)].fill(0);
        let height = (record + 1).trailing_zeros() as usize + 1;
        let position = (record + 1) >> height;
        for tree in [ENTRY, REVEAL] {
            self.set_slot(data, tree, record, 0, &self.zeros[self.levels]);
            if height < self.upper {
                let left = self.node(data, tree, height - 1, 2 * position);
                let node = pair(&left, &self.zeros[self.levels + height - 1]);
                self.set_slot(data, tree, record, 1, &node);
            }
        }
    }

    /// Folds a chunk node up the stored nodes and returns the root it implies.
    fn climb(&self, data: &[u8], tree: usize, chunk: usize, node: &Node) -> Node {
        let mut node = *node;
        let mut cursor = chunk;
        for height in 0..self.upper {
            let sibling = self.node(data, tree, height, cursor ^ 1);
            node = if cursor & 1 == 0 { pair(&node, &sibling) } else { pair(&sibling, &node) };
            cursor >>= 1;
        }
        node
    }

    /// Replaces a chunk node and rewrites every stored node above it. The instructions reach
    /// this through `append` and `reveal`. It is public so a test can build a large tree
    /// without hashing every leaf.
    pub fn set_chunk(&self, data: &mut [u8], tree: usize, chunk: usize, node: &Node) {
        self.set_slot(data, tree, chunk, 0, node);
        let mut node = *node;
        let mut cursor = chunk;
        for height in 0..self.upper {
            let sibling = self.node(data, tree, height, cursor ^ 1);
            node = if cursor & 1 == 0 { pair(&node, &sibling) } else { pair(&sibling, &node) };
            cursor >>= 1;
            if height + 1 == self.upper {
                self.set_root(data, tree, &node);
            } else {
                let home = Self::home(height + 1, cursor);
                if home < self.records {
                    self.set_slot(data, tree, home, 1, &node);
                }
            }
        }
    }

    /// True when `leaf` is the leaf at `index`. The proof is the bottom `levels` hashes.
    pub fn has_leaf(&self, data: &[u8], tree: usize, index: u32, leaf: &Node, proof: &[Node]) -> bool {
        let chunk = (index >> self.levels) as usize;
        if proof.len() != self.levels || chunk >= self.records {
            return false;
        }
        self.climb(data, tree, chunk, &fold(index, leaf, proof)) == self.root(data, tree)
    }

    /// Adds the next entry leaf and returns its index. The record for its chunk must exist.
    pub fn append(&self, data: &mut [u8], leaf: &Node) -> Option<u32> {
        let index = Self::count(data);
        let chunk = (index >> self.levels) as usize;
        if chunk >= self.records {
            return None;
        }
        let mut node = *leaf;
        let mut cursor = index;
        for level in 0..self.levels {
            let at = HEADER + 32 * level;
            if cursor & 1 == 0 {
                data[at..at + 32].copy_from_slice(&node);
                node = pair(&node, &self.zeros[level]);
            } else {
                let left: Node = data[at..at + 32].try_into().unwrap();
                node = pair(&left, &node);
            }
            cursor >>= 1;
        }
        self.set_chunk(data, ENTRY, chunk, &node);
        data[COUNT_AT..COUNT_AT + 4].copy_from_slice(&(index + 1).to_le_bytes());
        Some(index)
    }

    /// Writes the reveal leaf of an entered coupon. The reveal leaf must still be empty.
    pub fn reveal(&self, data: &mut [u8], index: u32, entry: &Node, entry_proof: &[Node], leaf: &Node, reveal_proof: &[Node]) -> bool {
        if index >= Self::count(data) || *leaf == self.zeros[0] {
            return false;
        }
        if !self.has_leaf(data, ENTRY, index, entry, entry_proof) {
            return false;
        }
        if !self.has_leaf(data, REVEAL, index, &self.zeros[0], reveal_proof) {
            return false;
        }
        let chunk = (index >> self.levels) as usize;
        self.set_chunk(data, REVEAL, chunk, &fold(index, leaf, reveal_proof));
        let reveals = Self::reveal_count(data) + 1;
        data[REVEALS_AT..REVEALS_AT + 4].copy_from_slice(&reveals.to_le_bytes());
        true
    }

    pub fn is_marked(&self, data: &[u8], index: u32) -> bool {
        let chunk = (index >> self.levels) as usize;
        let bit = (index as usize) & ((1usize << self.levels) - 1);
        chunk < self.records && data[self.record_at(chunk) + bit / 8] & (1 << (bit % 8)) != 0
    }

    /// Marks a coupon as paid or refunded. False when it already was, or it does not exist.
    pub fn mark(&self, data: &mut [u8], index: u32) -> bool {
        if index >= Self::count(data) || self.is_marked(data, index) {
            return false;
        }
        let chunk = (index >> self.levels) as usize;
        let bit = (index as usize) & ((1usize << self.levels) - 1);
        data[self.record_at(chunk) + bit / 8] |= 1 << (bit % 8);
        true
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct Rng(u64);

    impl Rng {
        fn next(&mut self) -> u64 {
            self.0 ^= self.0 << 13;
            self.0 ^= self.0 >> 7;
            self.0 ^= self.0 << 17;
            self.0
        }

        fn below(&mut self, bound: usize) -> usize {
            (self.next() % bound as u64) as usize
        }
    }

    fn leaf_of(tag: u8, index: u32) -> Node {
        hashv(&[&[tag], &index.to_le_bytes()]).to_bytes()
    }

    /// Every level of the full tree, built the slow way. `levels[0]` is the leaves.
    fn naive(leaves: &[Node], depth: usize) -> Vec<Vec<Node>> {
        let mut level: Vec<Node> = (0..1usize << depth).map(|index| leaves.get(index).copied().unwrap_or_else(|| crate::slip_zero(0))).collect();
        let mut all = vec![level.clone()];
        for _ in 0..depth {
            level = level.chunks(2).map(|both| pair(&both[0], &both[1])).collect();
            all.push(level.clone());
        }
        all
    }

    fn bottom(tree: &[Vec<Node>], index: usize, levels: usize) -> Vec<Node> {
        (0..levels).map(|level| tree[level][(index >> level) ^ 1]).collect()
    }

    struct Model {
        levels: usize,
        depth: usize,
        data: Vec<u8>,
        entries: Vec<Node>,
        reveals: Vec<Node>,
    }

    impl Model {
        fn new(levels: usize, depth: usize) -> Self {
            let mut data = vec![0u8; account_len(levels, 1)];
            Canopy::init(&mut data, &[9; 32], levels, 254, depth).unwrap();
            Self { levels, depth, data, entries: vec![], reveals: vec![] }
        }

        fn place(&mut self) -> u32 {
            let index = self.entries.len() as u32;
            if (index >> self.levels) as usize >= Canopy::open(&self.data, self.depth).unwrap().records() {
                assert!(Canopy::open(&self.data, self.depth).unwrap().append(&mut self.data, &[1; 32]).is_none(), "a missing record refuses the coupon");
                let grown = self.data.len() + record_len(self.levels);
                self.data.resize(grown, 0xff);
                let canopy = Canopy::open(&self.data, self.depth).unwrap();
                canopy.init_record(&mut self.data, canopy.records() - 1);
            }
            let leaf = leaf_of(1, index);
            let canopy = Canopy::open(&self.data, self.depth).unwrap();
            assert_eq!(canopy.append(&mut self.data, &leaf), Some(index));
            self.entries.push(leaf);
            self.reveals.push(crate::slip_zero(0));
            index
        }

        fn check(&self) {
            let canopy = Canopy::open(&self.data, self.depth).unwrap();
            let entries = naive(&self.entries, self.depth);
            let reveals = naive(&self.reveals, self.depth);
            assert_eq!(canopy.root(&self.data, ENTRY), entries[self.depth][0], "entry root at {} coupons", self.entries.len());
            assert_eq!(canopy.root(&self.data, REVEAL), reveals[self.depth][0], "reveal root at {} coupons", self.entries.len());
            let upper = self.depth - self.levels;
            for height in 0..=upper {
                for position in 0..1usize << (upper - height) {
                    assert_eq!(canopy.node(&self.data, ENTRY, height, position), entries[self.levels + height][position], "entry node {height}/{position}");
                    assert_eq!(canopy.node(&self.data, REVEAL, height, position), reveals[self.levels + height][position], "reveal node {height}/{position}");
                }
            }
            for index in 0..self.entries.len() {
                let proof = bottom(&entries, index, self.levels);
                assert!(canopy.has_leaf(&self.data, ENTRY, index as u32, &self.entries[index], &proof));
                assert!(!canopy.has_leaf(&self.data, ENTRY, index as u32, &leaf_of(3, index as u32), &proof));
                assert!(canopy.has_leaf(&self.data, REVEAL, index as u32, &self.reveals[index], &bottom(&reveals, index, self.levels)));
            }
        }

        fn reveal(&mut self, index: usize) {
            let canopy = Canopy::open(&self.data, self.depth).unwrap();
            let entries = naive(&self.entries, self.depth);
            let reveals = naive(&self.reveals, self.depth);
            let entry_proof = bottom(&entries, index, self.levels);
            let reveal_proof = bottom(&reveals, index, self.levels);
            let leaf = leaf_of(2, index as u32);
            let was_empty = self.reveals[index] == crate::slip_zero(0);
            let before = self.data.clone();
            assert!(!canopy.reveal(&mut self.data, index as u32, &leaf_of(3, index as u32), &entry_proof, &leaf, &reveal_proof), "an entry that was never placed cannot be revealed");
            assert!(!canopy.reveal(&mut self.data, index as u32, &self.entries[index], &entry_proof[1..], &leaf, &reveal_proof), "a short proof is refused");
            assert_eq!(canopy.reveal(&mut self.data, index as u32, &self.entries[index], &entry_proof, &leaf, &reveal_proof), was_empty);
            if was_empty {
                self.reveals[index] = leaf;
            } else {
                assert_eq!(before, self.data, "a refused reveal writes nothing");
            }
        }
    }

    #[test]
    fn small_trees_match_the_full_tree_in_any_order() {
        for (levels, depth, seed) in [(1usize, 4usize, 1u64), (2, 5, 2), (3, 6, 3), (2, 7, 4), (4, 6, 5)] {
            let mut rng = Rng(0x9e37_79b9_7f4a_7c15 ^ seed);
            let mut model = Model::new(levels, depth);
            model.check();
            let capacity = 1usize << depth;
            while model.entries.len() < capacity {
                if model.entries.is_empty() || rng.below(3) != 0 {
                    model.place();
                } else {
                    let index = rng.below(model.entries.len());
                    model.reveal(index);
                }
                if rng.below(4) == 0 || model.entries.len() == capacity {
                    model.check();
                }
            }
            for index in 0..capacity {
                model.reveal(index);
            }
            model.check();
            let canopy = Canopy::open(&model.data, depth).unwrap();
            assert_eq!(Canopy::count(&model.data) as usize, capacity);
            assert_eq!(Canopy::reveal_count(&model.data) as usize, capacity);
            assert_eq!(canopy.records(), 1 << (depth - levels));
            assert!(canopy.append(&mut model.data, &[1; 32]).is_none(), "a full tree takes no more coupons");
        }
    }

    #[test]
    fn the_live_shape_keeps_the_depth_twenty_root() {
        let (levels, depth) = (10usize, 20usize);
        let mut data = vec![0u8; account_len(levels, 1)];
        Canopy::init(&mut data, &[4; 32], levels, 255, depth).unwrap();
        assert_eq!(data.len(), 704);
        assert_eq!(record_len(levels), 256);
        assert_eq!(Canopy::open(&data, depth).unwrap().root(&data, ENTRY), crate::slip_empty_root());
        let mut leaves = Vec::new();
        let sparse_root = |leaves: &[Node]| {
            let mut level = leaves.to_vec();
            for height in 0..depth {
                level = (0..level.len().div_ceil(2))
                    .map(|at| pair(&level[2 * at], level.get(2 * at + 1).unwrap_or(&crate::slip_zero(height))))
                    .collect();
            }
            level[0]
        };
        for index in 0..2_600u32 {
            if (index >> levels) as usize >= Canopy::open(&data, depth).unwrap().records() {
                data.resize(data.len() + 256, 0);
                let canopy = Canopy::open(&data, depth).unwrap();
                canopy.init_record(&mut data, canopy.records() - 1);
            }
            let leaf = leaf_of(1, index);
            let canopy = Canopy::open(&data, depth).unwrap();
            assert_eq!(canopy.append(&mut data, &leaf), Some(index));
            leaves.push(leaf);
            if [0, 1, 1023, 1024, 1025, 2047, 2048, 2599].contains(&index) {
                assert_eq!(canopy.root(&data, ENTRY), sparse_root(&leaves), "root after coupon {index}");
            }
        }
        // The same leaf opens with a full depth-20 proof against this root, as on an older event.
        let canopy = Canopy::open(&data, depth).unwrap();
        assert_eq!(canopy.records(), 3);
        assert_eq!(data.len(), 704 + 2 * 256);
        let index = 1500usize;
        let mut level = leaves.clone();
        let mut full = Vec::new();
        let mut cursor = index;
        for height in 0..depth {
            full.push(level.get(cursor ^ 1).copied().unwrap_or_else(|| crate::slip_zero(height)));
            level = (0..level.len().div_ceil(2))
                .map(|at| pair(&level[2 * at], level.get(2 * at + 1).unwrap_or(&crate::slip_zero(height))))
                .collect();
            cursor >>= 1;
        }
        assert_eq!(crate::slip_fold(index as u32, &leaves[index], &full), canopy.root(&data, ENTRY));
        assert!(canopy.has_leaf(&data, ENTRY, index as u32, &leaves[index], &full[..levels]));
        assert!(!canopy.has_leaf(&data, ENTRY, index as u32, &leaves[index], &full), "only the bottom hashes are accepted");
    }

    #[test]
    fn a_coupon_is_marked_once() {
        let mut model = Model::new(2, 5);
        for _ in 0..9 {
            model.place();
        }
        let canopy = Canopy::open(&model.data, 5).unwrap();
        let roots = (canopy.root(&model.data, ENTRY), canopy.root(&model.data, REVEAL));
        assert!(!canopy.mark(&mut model.data, 9), "a coupon that was never placed cannot be marked");
        for index in [0u32, 3, 4, 8] {
            assert!(!canopy.is_marked(&model.data, index));
            assert!(canopy.mark(&mut model.data, index));
            assert!(canopy.is_marked(&model.data, index));
            assert!(!canopy.mark(&mut model.data, index), "a second mark is refused");
        }
        for index in [1u32, 2, 5, 6, 7] {
            assert!(!canopy.is_marked(&model.data, index));
        }
        assert_eq!(roots, (canopy.root(&model.data, ENTRY), canopy.root(&model.data, REVEAL)), "marks do not touch the trees");
        model.check();
    }

    #[test]
    fn a_wrong_account_is_not_opened() {
        let mut data = vec![0u8; account_len(10, 1)];
        assert!(Canopy::open(&data, 20).is_none(), "an account with no tag is refused");
        Canopy::init(&mut data, &[1; 32], 10, 250, 20).unwrap();
        assert!(Canopy::init(&mut data, &[1; 32], 10, 250, 20).is_none(), "a written account is not initialised again");
        assert_eq!(Canopy::event(&data), [1; 32]);
        data.push(0);
        assert!(Canopy::open(&data, 20).is_none(), "a length between two records is refused");
    }
}
