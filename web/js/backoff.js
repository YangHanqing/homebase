/* nextDelayMs: attempt 0 → 1s, then 2, 4, 8, 16, cap 30s. */
function homebaseNextDelayMs(attempt) {
  if (attempt < 0) {
    attempt = 0;
  }
  return Math.min(30000, 1000 * Math.pow(2, attempt));
}
