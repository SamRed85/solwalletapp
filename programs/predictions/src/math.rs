//! Payout arithmetic shared by the program and its host tests.
//! The formulas match docs/rules-spec.md.

pub const MAX_GAMES: usize = 13;
pub const MAX_COMBINATIONS: u32 = 44_000;
/// New SOL events. 5% of sales is the house share and 5% funds the league vault.
pub const SOL_TIER_TOTAL: u8 = 90;
/// Events created before the league vault. They do not fund it.
pub const LEGACY_TIER_TOTAL: u8 = 95;
/// League events. The pool was already charged when it was collected.
pub const LEAGUE_TIER_TOTAL: u8 = 100;
pub const TIER_PERCENT_TOTAL: u8 = SOL_TIER_TOTAL;
/// A winning row paid less than this rolls its whole tier into the next event.
pub const MIN_ROW_PAYOUT: u64 = 10_000_000;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Tier {
    pub correct: u8,
    pub percent: u8,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Settlement {
    pub house: u64,
    pub paid: u64,
    pub rollover_out: u64,
    pub per_winner: Vec<u64>,
}

pub const VOID_RESULT: u8 = b'C';

pub fn mask_for(outcome: u8) -> Option<u8> {
    match outcome {
        b'1' => Some(0b001),
        b'X' => Some(0b010),
        b'2' => Some(0b100),
        _ => None,
    }
}

/// A voided game scores every pick as correct, so it changes no coupon's relative standing.
pub fn result_mask(result: u8) -> Option<u8> {
    if result == VOID_RESULT {
        return Some(0b111);
    }
    mask_for(result)
}

pub fn combination_count(picks: &[u8]) -> Result<u32, &'static str> {
    let mut combinations = 1u32;
    for &mask in picks {
        if mask == 0 || mask > 0b111 {
            return Err("every game needs one or more of 1, X, 2");
        }
        let choices = mask.count_ones();
        if combinations > MAX_COMBINATIONS / choices {
            return Err("too many combinations");
        }
        combinations *= choices;
    }
    Ok(combinations)
}

/// Rows of one system entry by number of correct picks. Index `n` holds the rows with
/// exactly `n` correct; indexes above the game count stay zero.
pub fn correct_counts(picks: &[u8], results: &[u8]) -> Result<[u32; MAX_GAMES + 1], &'static str> {
    if picks.len() != results.len() || picks.len() > MAX_GAMES {
        return Err("result count does not match the games");
    }
    let mut counts = [0u32; MAX_GAMES + 1];
    counts[0] = 1;
    for (game, (&mask, &result)) in picks.iter().zip(results).enumerate() {
        let result_bits = result_mask(result).ok_or("result must be 1, X, 2 or C")?;
        let choices = (mask & 0b111).count_ones();
        let correct = (mask & result_bits).count_ones();
        let wrong = choices - correct;
        let mut next = [0u32; MAX_GAMES + 1];
        for score in 0..=game {
            let rows = counts[score];
            if rows == 0 {
                continue;
            }
            next[score] = next[score].checked_add(rows.checked_mul(wrong).ok_or("row count overflows")?).ok_or("row count overflows")?;
            next[score + 1] = next[score + 1].checked_add(rows.checked_mul(correct).ok_or("row count overflows")?).ok_or("row count overflows")?;
        }
        counts = next;
    }
    Ok(counts)
}

pub const MAX_COLORS: usize = 4;
/// Largest colored count table, not counting the correct-pick dimension. It is checked when a
/// coupon is revealed, so scoring that coupon later always fits. Never lower it with a coupon
/// still unscored: a revealed coupon over a lower cap could not be counted.
pub const MAX_COLOR_STATES: usize = 64;

/// One reduction color. `signs` has three bits per game in pick-mask order (1, X, 2),
/// so game `g` outcome bit `k` is bit `3 * g + k`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Color {
    pub signs: u64,
    pub min: u8,
    pub max: u8,
}

#[derive(Clone, Copy, Default)]
struct ActiveColor {
    signs: u64,
    min: u8,
    cap: u8,
    stride: usize,
}

fn sign_mask(game: usize) -> u64 {
    0b111u64 << (3 * game)
}

/// Rows of one coupon by number of correct picks. With no colors this is `correct_counts`
/// and the frame must fit in 44,000 rows. With colors only the rows inside every color's
/// range are counted, and the rows are never listed. `results` of `None` puts every kept
/// row at zero correct, so the sum of the result is the kept row count.
pub fn kept_counts(picks: &[u8], colors: &[Color], results: Option<&[u8]>) -> Result<[u32; MAX_GAMES + 1], &'static str> {
    if picks.len() > MAX_GAMES {
        return Err("too many games");
    }
    if let Some(results) = results {
        if results.len() != picks.len() {
            return Err("result count does not match the games");
        }
    }
    if colors.is_empty() {
        combination_count(picks)?;
        return match results {
            Some(results) => correct_counts(picks, results),
            None => {
                let mut counts = [0u32; MAX_GAMES + 1];
                counts[0] = combination_count(picks)?;
                Ok(counts)
            }
        };
    }
    if colors.len() > MAX_COLORS {
        return Err("at most 4 colors");
    }
    let mut frame = 0u64;
    for (game, &mask) in picks.iter().enumerate() {
        if mask == 0 || mask > 0b111 {
            return Err("every game needs one or more of 1, X, 2");
        }
        frame |= u64::from(mask) << (3 * game);
    }
    let mut active = [ActiveColor::default(); MAX_COLORS];
    let mut active_count = 0usize;
    let mut states = 1usize;
    let mut impossible = false;
    for color in colors {
        if color.signs == 0 || color.signs & !frame != 0 {
            return Err("a color sign must be one of the coupon's picks");
        }
        if color.min > color.max || color.max as usize > picks.len() {
            return Err("a color needs a min that is not above its max");
        }
        let reach = (0..picks.len()).filter(|game| color.signs & sign_mask(*game) != 0).count() as u8;
        if color.min > reach {
            impossible = true;
            continue;
        }
        let cap = color.max.min(reach);
        if color.min == 0 && cap == reach {
            continue;
        }
        active[active_count] = ActiveColor { signs: color.signs, min: color.min, cap, stride: states };
        active_count += 1;
        states = states.checked_mul(cap as usize + 1).ok_or("too many color states")?;
        if states > MAX_COLOR_STATES {
            return Err("too many color states");
        }
    }
    let mut counts = [0u32; MAX_GAMES + 1];
    if impossible {
        return Ok(counts);
    }
    let active = &active[..active_count];
    // Per state, which colors are at their cap and which are still below their min.
    let mut at_cap = [0u8; MAX_COLOR_STATES];
    let mut below_min = [0u8; MAX_COLOR_STATES];
    for state in 0..states {
        for (index, color) in active.iter().enumerate() {
            let digit = (state / color.stride) % (color.cap as usize + 1);
            if digit == color.cap as usize {
                at_cap[state] |= 1 << index;
            }
            if digit < color.min as usize {
                below_min[state] |= 1 << index;
            }
        }
    }
    // Without results every row stays at zero correct, so one score slot is enough.
    let width = if results.is_some() { picks.len() + 1 } else { 1 };
    // Each cell is at most the frame's row count, 3^13, so the additions below cannot overflow.
    let mut current = vec![0u32; states * width];
    let mut next = vec![0u32; states * width];
    current[0] = 1;
    let mut live = 1u64;
    for (game, &mask) in picks.iter().enumerate() {
        let result_bits = match results {
            Some(results) => result_mask(results[game]).ok_or("result must be 1, X, 2 or C")?,
            None => 0,
        };
        let scores = if width == 1 { 1 } else { game + 1 };
        next.fill(0);
        let mut next_live = 0u64;
        for outcome in 0..3usize {
            if mask & (1 << outcome) == 0 {
                continue;
            }
            let bit = 1u64 << (3 * game + outcome);
            let mut touched = 0u8;
            let mut step = 0usize;
            for (index, color) in active.iter().enumerate() {
                if color.signs & bit != 0 {
                    touched |= 1 << index;
                    step += color.stride;
                }
            }
            let shift = usize::from(width > 1 && result_bits & (1 << outcome) != 0);
            let mut pending = live;
            while pending != 0 {
                let state = pending.trailing_zeros() as usize;
                pending &= pending - 1;
                if at_cap[state] & touched != 0 {
                    continue;
                }
                let target = state + step;
                next_live |= 1 << target;
                let from = &current[state * width..state * width + scores];
                let to = &mut next[target * width + shift..target * width + shift + scores];
                for (cell, rows) in to.iter_mut().zip(from) {
                    *cell += *rows;
                }
            }
        }
        core::mem::swap(&mut current, &mut next);
        live = next_live;
    }
    for state in 0..states {
        if live & (1 << state) == 0 || below_min[state] != 0 {
            continue;
        }
        for (total, rows) in counts.iter_mut().zip(&current[state * width..(state + 1) * width]) {
            *total = total.checked_add(*rows).ok_or("row count overflows")?;
        }
    }
    Ok(counts)
}

/// Kept rows of one coupon, from `kept_counts` with no results.
pub fn kept_rows(picks: &[u8], colors: &[Color]) -> Result<u32, &'static str> {
    let counts = kept_counts(picks, colors, None)?;
    let mut total = 0u32;
    for rows in counts {
        total = total.checked_add(rows).ok_or("row count overflows")?;
    }
    Ok(total)
}

pub fn tier_total(tiers: &[Tier]) -> Result<u8, &'static str> {
    let mut total = 0u8;
    for tier in tiers {
        total = total.checked_add(tier.percent).ok_or("tier percentages overflow")?;
    }
    Ok(total)
}

pub fn validate_tiers(tiers: &[Tier], game_count: u8) -> Result<(), &'static str> {
    validate_tiers_total(tiers, game_count, tier_total(tiers)?)
}

pub fn validate_tiers_total(tiers: &[Tier], game_count: u8, expected: u8) -> Result<(), &'static str> {
    if tiers.is_empty() || tiers.len() > MAX_GAMES {
        return Err("at least one prize tier is required");
    }
    let mut seen = [false; MAX_GAMES + 1];
    let mut total = 0u8;
    let mut has_top = false;
    for tier in tiers {
        if tier.correct == 0 || tier.correct > game_count {
            return Err("tier correct picks must be between 1 and the number of games");
        }
        if seen[tier.correct as usize] {
            return Err("each number of correct picks can only have one tier");
        }
        if tier.percent == 0 {
            return Err("tier percentages must be positive");
        }
        seen[tier.correct as usize] = true;
        total = total.checked_add(tier.percent).ok_or("tier percentages overflow")?;
        has_top = has_top || tier.correct == game_count;
    }
    if !has_top {
        return Err("there must be a tier for all games correct");
    }
    if total != expected {
        return Err("tier percentages do not add up");
    }
    if expected != SOL_TIER_TOTAL && expected != LEGACY_TIER_TOTAL && expected != LEAGUE_TIER_TOTAL {
        return Err("tier percentages must add up to 90, 95, or 100");
    }
    Ok(())
}

pub fn settle(sales: u64, rollover_in: u64, game_count: u8, tiers: &[Tier], winners: &[u64]) -> Result<Settlement, &'static str> {
    validate_tiers(tiers, game_count)?;
    if winners.len() < game_count as usize + 1 {
        return Err("winner table does not cover every score");
    }
    let mut settlement = Settlement {
        house: 0,
        paid: 0,
        rollover_out: 0,
        per_winner: vec![0; tiers.len()],
    };
    let mut allocated = 0u64;
    for (index, tier) in tiers.iter().enumerate() {
        let mut pot = sales as u128 * u128::from(tier.percent) / 100;
        allocated += u64::try_from(pot).map_err(|_| "tier pot overflows")?;
        if tier.correct == game_count {
            pot += u128::from(rollover_in);
        }
        let pot = u64::try_from(pot).map_err(|_| "tier pot overflows")?;
        let winner_count = winners[tier.correct as usize];
        let per_winner = if winner_count == 0 { 0 } else { pot / winner_count };
        if winner_count == 0 || per_winner < MIN_ROW_PAYOUT {
            settlement.rollover_out = settlement.rollover_out.checked_add(pot).ok_or("rollover overflows")?;
        } else {
            let paid = per_winner * winner_count;
            settlement.per_winner[index] = per_winner;
            settlement.paid = settlement.paid.checked_add(paid).ok_or("payout overflows")?;
            settlement.house = settlement.house.checked_add(pot - paid).ok_or("house amount overflows")?;
        }
    }
    settlement.house = settlement.house.checked_add(sales - allocated).ok_or("house amount overflows")?;
    if settlement.house + settlement.paid + settlement.rollover_out != sales + rollover_in {
        return Err("settlement does not conserve funds");
    }
    Ok(settlement)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn standard_tiers() -> Vec<Tier> {
        vec![Tier { correct: 13, percent: 50 }, Tier { correct: 12, percent: 25 }, Tier { correct: 11, percent: 15 }, Tier { correct: 10, percent: 5 }]
    }

    fn row(text: &str) -> Vec<u8> {
        text.bytes().map(|outcome| mask_for(outcome).unwrap()).collect()
    }

    fn winners_for(rows: &[&str], results: &str) -> Vec<u64> {
        let mut winners = vec![0u64; results.len() + 1];
        for row_text in rows {
            let counts = correct_counts(&row(row_text), &results.as_bytes()).unwrap();
            for (score, winner) in winners.iter_mut().enumerate() {
                *winner += u64::from(counts[score]);
            }
        }
        winners
    }

    #[test]
    fn settlement_matches_the_shared_pool_rules() {
        let results = "1111111111111";
        let rows = ["1111111111111", "1111111111112", "1111111111122", "111111111112X", "2222222222222"];
        // Scaled so every paid row stays at or above 0.01 SOL. The 10-correct tier still has no winner.
        let scale = 400;
        let settlement = settle(5_000_000 * scale, 2_000_000 * scale, 13, &standard_tiers(), &winners_for(&rows, results)).unwrap();
        assert_eq!(settlement.per_winner, vec![4_500_000 * scale, 1_250_000 * scale, 375_000 * scale, 0]);
        assert_eq!(settlement.rollover_out, 250_000 * scale);
        assert_eq!(settlement.house, 250_000 * scale);
        assert_eq!(settlement.paid, (4_500_000 + 1_250_000 + 375_000 * 2) * scale);
    }

    #[test]
    fn unwon_jackpot_rolls_over_without_a_second_house_cut() {
        let winners = winners_for(&["2222222222222"], "1111111111111");
        let settlement = settle(1_000_000, 7_000_000, 13, &standard_tiers(), &winners).unwrap();
        assert_eq!(settlement.paid, 0);
        assert_eq!(settlement.rollover_out, 950_000 + 7_000_000);
        assert_eq!(settlement.house, 50_000);
    }

    #[test]
    fn rounding_remainder_goes_to_the_house() {
        let tiers = [Tier { correct: 7, percent: 95 }];
        let winners = winners_for(&["1111111", "1111111", "1111111"], "1111111");
        let sales = 400_000_001;
        let settlement = settle(sales, 0, 7, &tiers, &winners).unwrap();
        let pot = sales * 95 / 100;
        assert!(settlement.per_winner[0] >= MIN_ROW_PAYOUT);
        assert_eq!(settlement.per_winner, vec![pot / 3]);
        assert!(settlement.paid < pot);
        assert_eq!(settlement.house, sales - settlement.paid);
    }

    #[test]
    fn a_share_under_a_hundredth_of_a_sol_rolls_the_whole_tier() {
        let tiers = [Tier { correct: 7, percent: 95 }];
        let mut winners = vec![0u64; 8];
        winners[7] = 10;
        let settlement = settle(100_000_000, 0, 7, &tiers, &winners).unwrap();
        assert_eq!(settlement.per_winner, vec![0]);
        assert_eq!(settlement.paid, 0);
        assert_eq!(settlement.rollover_out, 95_000_000);
        assert_eq!(settlement.house, 5_000_000);

        winners[7] = 1;
        let exact = settle(10_526_316, 0, 7, &tiers, &winners).unwrap();
        assert_eq!(exact.per_winner, vec![MIN_ROW_PAYOUT]);
        assert_eq!(exact.paid, MIN_ROW_PAYOUT);
        assert_eq!(exact.rollover_out, 0);
    }

    #[test]
    fn empty_event_passes_rollover_through() {
        let settlement = settle(0, 5_000, 13, &standard_tiers(), &vec![0; 14]).unwrap();
        assert_eq!(settlement.rollover_out, 5_000);
        assert_eq!(settlement.paid, 0);
    }

    #[test]
    fn system_counts_match_expanded_rows() {
        let picks = [0b011, 0b101, 0b001];
        let counts = correct_counts(&picks, b"111").unwrap();
        let mut expanded = [0u32; 4];
        for row_text in ["111", "121", "X11", "X21"] {
            let score = row(row_text)
                .iter()
                .zip(b"111")
                .filter(|(pick, result)| *pick & mask_for(**result).unwrap() != 0)
                .count();
            expanded[score] += 1;
        }
        assert_eq!(&counts[..4], &expanded);
        assert!(counts[4..].iter().all(|rows| *rows == 0));
        assert_eq!(combination_count(&picks).unwrap(), 4);
    }

    #[test]
    fn void_game_scores_every_pick_as_correct() {
        let picks = [0b011, 0b001, 0b100];
        let counts = correct_counts(&picks, b"C12").unwrap();
        assert_eq!(&counts[..4], &[0, 0, 0, 2]);
        let counts = correct_counts(&picks, b"C21").unwrap();
        assert_eq!(&counts[..4], &[0, 2, 0, 0]);
        let unrevealed = correct_counts(&[0, 0, 0], b"C12").unwrap();
        assert!(unrevealed.iter().all(|rows| *rows == 0), "an unrevealed coupon has no rows");
        assert!(correct_counts(&picks, b"Z12").is_err());
    }

    fn brute_kept(picks: &[u8], colors: &[Color], results: &[u8]) -> [u32; MAX_GAMES + 1] {
        let mut counts = [0u32; MAX_GAMES + 1];
        let mut row: Vec<u32> = picks.iter().map(|mask| mask.trailing_zeros()).collect();
        loop {
            let mut kept = true;
            for color in colors {
                let hits = row.iter().enumerate().filter(|(game, outcome)| color.signs >> (3 * *game as u32 + **outcome) & 1 == 1).count();
                kept &= hits >= color.min as usize && hits <= color.max as usize;
            }
            if kept {
                let correct = row.iter().zip(results).filter(|(outcome, result)| result_mask(**result).unwrap() >> **outcome & 1 == 1).count();
                counts[correct] += 1;
            }
            let mut game = 0;
            loop {
                if game == picks.len() {
                    return counts;
                }
                let mut outcome = row[game] + 1;
                while outcome < 3 && picks[game] >> outcome & 1 == 0 {
                    outcome += 1;
                }
                if outcome < 3 {
                    row[game] = outcome;
                    break;
                }
                row[game] = picks[game].trailing_zeros();
                game += 1;
            }
        }
    }

    struct Random(u64);

    impl Random {
        fn next(&mut self) -> u64 {
            self.0 ^= self.0 << 13;
            self.0 ^= self.0 >> 7;
            self.0 ^= self.0 << 17;
            self.0
        }

        fn below(&mut self, bound: u64) -> u64 {
            self.next() % bound
        }
    }

    fn full_frame(games: usize) -> Vec<u8> {
        vec![0b111; games]
    }

    fn favorites(games: &[usize]) -> u64 {
        games.iter().map(|game| 1u64 << (3 * game)).sum()
    }

    #[test]
    fn colored_counts_match_every_row() {
        let mut random = Random(0x9e37_79b9_7f4a_7c15);
        let mut checked = 0;
        for _ in 0..600 {
            let games = 1 + random.below(7) as usize;
            let picks: Vec<u8> = (0..games).map(|_| 1 + random.below(7) as u8).collect();
            let frame: u64 = picks.iter().enumerate().map(|(game, mask)| u64::from(*mask) << (3 * game)).sum();
            let color_count = 1 + random.below(MAX_COLORS as u64) as usize;
            let mut colors = Vec::new();
            for _ in 0..color_count {
                let signs = frame & random.next();
                if signs == 0 {
                    continue;
                }
                let min = random.below(games as u64 + 1) as u8;
                let max = min + random.below(games as u64 + 1 - u64::from(min)) as u8;
                colors.push(Color { signs, min, max });
            }
            if colors.is_empty() {
                continue;
            }
            let results: Vec<u8> = (0..games).map(|_| b"1X2C"[random.below(4) as usize]).collect();
            let Ok(counts) = kept_counts(&picks, &colors, Some(&results)) else { continue };
            assert_eq!(counts, brute_kept(&picks, &colors, &results), "picks {picks:?} colors {colors:?} results {results:?}");
            let total: u32 = counts.iter().sum();
            assert_eq!(kept_rows(&picks, &colors).unwrap(), total);
            checked += 1;
        }
        assert!(checked > 300, "only {checked} random coupons were compared");
    }

    #[test]
    fn six_yellow_favorites_keep_the_worked_example_rows() {
        let colors = [Color { signs: favorites(&[0, 1, 2, 3, 4, 5]), min: 3, max: 5 }];
        assert_eq!(kept_rows(&full_frame(8), &colors).unwrap(), 2_088);
    }

    #[test]
    fn every_outcome_on_thirteen_games_needs_colors_to_fit() {
        let frame = full_frame(13);
        assert!(kept_counts(&frame, &[], None).is_err(), "1,594,323 rows do not fit one plain coupon");
        let all_ones = favorites(&(0..13).collect::<Vec<_>>());
        assert_eq!(kept_rows(&frame, &[Color { signs: all_ones, min: 13, max: 13 }]).unwrap(), 1);
        assert_eq!(kept_rows(&frame, &[Color { signs: all_ones, min: 0, max: 0 }]).unwrap(), 8_192);
        assert_eq!(kept_rows(&frame, &[Color { signs: all_ones, min: 0, max: 13 }]).unwrap(), 1_594_323);
        let one_two = favorites(&(0..13).collect::<Vec<_>>()) * 0b101;
        let counts = kept_counts(&frame, &[Color { signs: one_two, min: 13, max: 13 }], Some(b"1111111111111")).unwrap();
        assert_eq!(counts.iter().sum::<u32>(), 8_192);
        assert_eq!(counts[13], 1, "only the all-1 row is fully correct");
    }

    #[test]
    fn a_min_no_row_can_reach_keeps_nothing() {
        let colors = [Color { signs: 0b011, min: 2, max: 2 }];
        assert_eq!(kept_rows(&[0b011, 0b001], &colors).unwrap(), 0, "a row hits game 0 once at most");
    }

    #[test]
    fn colors_that_do_not_match_the_coupon_are_rejected() {
        let picks = [0b011, 0b001, 0b100];
        assert!(kept_rows(&picks, &[Color { signs: 0b100, min: 0, max: 1 }]).is_err(), "2 is not picked on game 0");
        assert!(kept_rows(&picks, &[Color { signs: 0, min: 0, max: 1 }]).is_err(), "a color needs a sign");
        assert!(kept_rows(&picks, &[Color { signs: 0b001, min: 2, max: 1 }]).is_err(), "min above max");
        assert!(kept_rows(&picks, &[Color { signs: 0b001, min: 0, max: 4 }]).is_err(), "max above the game count");
        let five = [Color { signs: 0b001, min: 0, max: 1 }; 5];
        assert!(kept_rows(&picks, &five).is_err(), "at most four colors");
    }

    #[test]
    fn a_table_over_the_cap_is_refused() {
        let frame = full_frame(13);
        let ones = favorites(&(0..13).collect::<Vec<_>>());
        let wide = [Color { signs: ones, min: 1, max: 8 }, Color { signs: ones << 1, min: 1, max: 8 }];
        assert_eq!(kept_counts(&frame, &wide, None), Err("too many color states"));
        let tight = [Color { signs: ones, min: 1, max: 3 }, Color { signs: ones << 1, min: 1, max: 3 }];
        assert!(kept_counts(&frame, &tight, None).is_ok());
    }

    #[test]
    fn rejects_invalid_rules() {
        assert!(combination_count(&[0]).is_err());
        assert!(combination_count(&[0b111; 13]).is_err());
        assert!(validate_tiers(&[Tier { correct: 7, percent: 80 }], 7).is_err());
        assert!(validate_tiers(&[Tier { correct: 6, percent: 90 }], 7).is_err());
        assert!(validate_tiers_total(&[Tier { correct: 7, percent: 100 }], 7, LEAGUE_TIER_TOTAL).is_ok());
    }
}
