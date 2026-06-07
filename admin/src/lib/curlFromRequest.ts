function escapeSingleQuotes(s: string): string {
  return s.replace(/'/g, "'\\''");
}

export function curlFromRequest(
  method: string,
  url: string,
  headers: Array<{ key: string; value: string }>,
  body?: string | null
): string {
  const lines = [`curl -X ${method} '${escapeSingleQuotes(url)}'`];
  for (const h of headers) {
    lines.push(`  -H '${escapeSingleQuotes(h.key)}: ${escapeSingleQuotes(h.value)}'`);
  }
  if (body) {
    lines.push(`  --data-raw '${escapeSingleQuotes(body)}'`);
  }
  return lines.join(" \\\n");
}
