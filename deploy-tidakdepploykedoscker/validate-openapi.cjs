const fs = require("fs");

const specPath = process.argv[2];
if (!specPath) throw new Error("OpenAPI path argument required");

const doc = JSON.parse(fs.readFileSync(specPath, "utf8"));
if (doc.openapi !== "3.1.0") throw new Error("OpenAPI version must be 3.1.0");
if (!doc.paths || !doc.components?.securitySchemes?.ApiKey || !doc.components?.securitySchemes?.AdminBearer) {
  throw new Error("OpenAPI paths or security schemes missing");
}

console.log(`OpenAPI syntax OK: ${Object.keys(doc.paths).length} paths`);

