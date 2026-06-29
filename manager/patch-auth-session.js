"use strict";

const fs = require("node:fs");
const path = require("node:path");

const assetsDir = path.join(__dirname, "dist", "assets");
const bundleName = fs
  .readdirSync(assetsDir)
  .find((name) => /^index-.*\.js$/.test(name));

if (!bundleName) {
  throw new Error("manager JavaScript bundle not found");
}

const bundlePath = path.join(assetsDir, bundleName);
let source = fs.readFileSync(bundlePath, "utf8");

function replaceExact(label, before, after) {
  if (source.includes(after)) return;

  const first = source.indexOf(before);
  const last = source.lastIndexOf(before);
  if (first === -1) {
    throw new Error(`${label}: expected source fragment not found`);
  }
  if (first !== last) {
    throw new Error(`${label}: expected exactly one source fragment`);
  }
  source = source.slice(0, first) + after + source.slice(first + before.length);
}

replaceExact(
  "normalize store login key",
  'const s=n.replace(/\\\/$/,"");try{await Xt.get("/instance/all",{baseURL:s,headers:{apikey:r,"Cache-Control":"no-cache"},params:{t:Date.now()}}),t({apiUrl:s,apiKey:r,isAuthenticated:!0})',
  'const s=n.replace(/\\\/$/,""),o=r.trim();try{await Xt.get("/instance/all",{baseURL:s,headers:{apikey:o,"Cache-Control":"no-cache"},params:{t:Date.now()}}),t({apiUrl:s,apiKey:o,isAuthenticated:!0})',
);

replaceExact(
  "read normalized interceptor status",
  'const u=(i=c==null?void 0:c.response)==null?void 0:i.status;',
  'const u=(c==null?void 0:c.status)??((i=c==null?void 0:c.response)==null?void 0:i.status);',
);

replaceExact(
  "normalize submitted form key",
  'const j=A.apiUrl.replace(/\\\/$/,"");if(Ve.info("Verificando licenca...")',
  'const j=A.apiUrl.replace(/\\\/$/,""),R=A.apiKey.trim();if(Ve.info("Verificando licenca...")',
);

replaceExact(
  "use normalized key for license check",
  'await n(j,A.apiKey)!=="licensed"',
  'await n(j,R)!=="licensed"',
);

replaceExact(
  "persist normalized key",
  'r(j),s(A.apiKey),window.location.href=B.register_url',
  'r(j),s(R),window.location.href=B.register_url',
);

replaceExact(
  "use normalized key for login",
  'await t(A.apiUrl,A.apiKey),Ve.success',
  'await t(A.apiUrl,R),Ve.success',
);

fs.writeFileSync(bundlePath, source);
