//! Payout arithmetic shared by the program and its host tests.
//! The formulas match docs/rules-spec.md.

pub const MAX_GAMES: usize = 13;
pub const MAX_COMBINATIONS: u32 = 44_000;
pub const TIER_PERCENT_TOTAL: u8 = 95;

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

pub fn mask_for(outcome: u8) -> Option<u8> {
    match outcome {
        b'1' => Some(0b001),
        b'X' => Some(0b010),
        b'2' => Some(0b100),
        _ => None,
    }
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

pub fn correct_counts(picks: &[u8], results: &[u8]) -> Result<Vec<u32>, &'static str> {
    if picks.len() != results.len() {
        return Err("result count does not match the games");
    }
    let mut counts = vec![1u32];
    for (&mask, &result) in picks.iter().zip(results) {
        let result_bit = mask_for(result).ok_or("result must be 1, X, or 2")?;
        let choices = mask.count_ones();
        let correct = u32::from(mask & result_bit != 0);
        let wrong = choices - correct;
        let mut next = vec![0u32; counts.len() + 1];
        for (score, &rows) in counts.iter().enumerate() {
            if wrong > 0 {
                next[score] += rows * wrong;
            }
            if correct > 0 {
                next[score + 1] += rows * correct;
            }
        }
        counts = next;
    }
    Ok(counts)
}

pub fn validate_tiers(tiers: &[Tier], game_count: u8) -> Result<(), &'static str> {
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
    if total != TIER_PERCENT_TOTAL {
        return Err("tier percentages must add up to 95");
    }
    Ok(())
}

pub fn settle(sales: u64, rollover_in: u64, game_count: u8, tiers: &[Tier], winners: &[u32]) -> Result<Settlement, &'static str> {
    validate_tiers(tiers, game_count)?;
    if winners.len() != game_count as usize + 1 {
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
        let winner_count = u64::from(winners[tier.correct as usize]);
        if winner_count == 0 {
            settlement.rollover_out = settlement.rollover_out.checked_add(pot).ok_or("rollover overflows")?;
        } else {
            let per_winner = pot / winner_count;
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

    fn winners_for(rows: &[&str], results: &str) -> Vec<u32> {
        let mut winners = vec![0u32; results.len() + 1];
        for row_text in rows {
            let counts = correct_counts(&row(row_text), &results.as_bytes()).unwrap();
            for (score, count) in counts.iter().enumerate() {
                winners[score] += count;
            }
        }
        winners
    }

    #[test]
    fn settlement_matches_the_shared_pool_rules() {
        let results = "1111111111111";
        let rows = ["1111111111111", "1111111111112", "1111111111122", "111111111112X", "2222222222222"];
        let settlement = settle(5_000_000, 2_000_000, 13, &standard_tiers(), &winners_for(&rows, results)).unwrap();
        assert_eq!(settlement.per_winner, vec![4_500_000, 1_250_000, 375_000, 0]);
        assert_eq!(settlement.rollover_out, 250_000);
        assert_eq!(settlement.house, 250_000);
        assert_eq!(settlement.paid, 4_500_000 + 1_250_000 + 375_000 * 2);
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
        let settlement = settle(1_000_001, 0, 7, &tiers, &winners).unwrap();
        assert_eq!(settlement.per_winner, vec![950_000 / 3]);
        assert!(settlement.paid <= 950_000);
        assert_eq!(settlement.house, 1_000_001 - settlement.paid);
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
        assert_eq!(counts, expanded);
        assert_eq!(combination_count(&picks).unwrap(), 4);
    }

    #[test]
    fn rejects_invalid_rules() {
        assert!(combination_count(&[0]).is_err());
        assert!(combination_count(&[0b111; 13]).is_err());
        assert!(validate_tiers(&[Tier { correct: 7, percent: 100 }], 7).is_err());
        assert!(validate_tiers(&[Tier { correct: 6, percent: 95 }], 7).is_err());
    }
}
