export interface ParsedCurl {
  method: string;
  url: string;
  headers: Array<{ key: string; value: string }>;
  body: string;
}

export function parseCurl(input: string): ParsedCurl | null {
  const normalized = input.replace(/\\\n\s*/g, " ").trim();
  if (!normalized.startsWith("curl ") && normalized !== "curl") return null;

  const tokens = tokenize(normalized.slice(5));

  let method = "";
  let url = "";
  const headers: Array<{ key: string; value: string }> = [];
  let body = "";

  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i];
    if (t === "-X" || t === "--request") {
      method = tokens[++i] ?? "";
    } else if (t === "-H" || t === "--header") {
      const raw = tokens[++i] ?? "";
      const colon = raw.indexOf(":");
      if (colon !== -1) {
        headers.push({ key: raw.slice(0, colon).trim(), value: raw.slice(colon + 1).trim() });
      }
    } else if (t === "-d" || t === "--data" || t === "--data-raw" || t === "--data-binary") {
      body = tokens[++i] ?? "";
    } else if (!t.startsWith("-") && !url) {
      url = t;
    }
  }

  if (!url) return null;
  if (!method) method = body ? "POST" : "GET";
  return { method: method.toUpperCase(), url, headers, body };
}

function tokenize(input: string): string[] {
  const tokens: string[] = [];
  let i = 0;
  while (i < input.length) {
    while (i < input.length && /\s/.test(input[i])) i++;
    if (i >= input.length) break;

    const char = input[i];
    if (char === "'" || char === '"') {
      const quote = char;
      i++;
      let token = "";
      while (i < input.length && input[i] !== quote) {
        if (input[i] === "\\" && quote === '"') {
          i++;
          token += input[i] ?? "";
        } else {
          token += input[i];
        }
        i++;
      }
      i++;
      tokens.push(token);
    } else {
      let token = "";
      while (i < input.length && !/\s/.test(input[i])) {
        token += input[i++];
      }
      tokens.push(token);
    }
  }
  return tokens;
}
