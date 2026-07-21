// vs-ai2.59: self-check for the bot search-diagnostics formatters.
// Run: node tests/test_bot_diagnostics.js
const assert = require('assert');
const { formatBotEval, formatBotNodes, formatBotTime } = require('../multiplayer.js');

// eval: scaled /1000, signed; mate above 1e8 threshold.
assert.strictEqual(formatBotEval(1234), '+1.2');
assert.strictEqual(formatBotEval(-1234), '-1.2');
assert.strictEqual(formatBotEval(0), '+0.0');
assert.strictEqual(formatBotEval(1e9), '+mate');   // mateScore
assert.strictEqual(formatBotEval(-1e9), '−mate');

// nodes: 58k / 1.2M / raw.
assert.strictEqual(formatBotNodes(58000), '58k');
assert.strictEqual(formatBotNodes(1200000), '1.2M');
assert.strictEqual(formatBotNodes(742), '742');

// time: seconds (1 decimal) once past 100ms, raw ms only for very fast moves.
assert.strictEqual(formatBotTime(900), '0.9s');
assert.strictEqual(formatBotTime(1500), '1.5s');
assert.strictEqual(formatBotTime(45), '45ms');

console.log('bot diagnostics formatters OK');
