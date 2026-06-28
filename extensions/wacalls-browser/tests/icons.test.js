"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const root = path.join(__dirname, "..");

test("provides valid square PNG icons in every manifest size", () => {
  [16, 32, 48, 128].forEach((size) => {
    const data = fs.readFileSync(path.join(root, "icons", `icon-${size}.png`));
    assert.deepEqual([...data.subarray(0, 8)], [137, 80, 78, 71, 13, 10, 26, 10]);
    assert.equal(data.readUInt32BE(16), size);
    assert.equal(data.readUInt32BE(20), size);
  });
});
