export interface BulkInviteCsvRow {
  email: string;
  first_name: string;
  last_name: string;
  role: string;
  password: string;
  confirm_password: string;
}

// Splits a single CSV line into cells, honoring double-quoted fields (so a
// quoted field may contain commas) and "" as an escaped literal quote.
// Does not support quoted fields spanning multiple lines.
const parseCsvLine = (line: string): string[] => {
  const cells: string[] = [];
  let current = '';
  let inQuotes = false;

  for (let i = 0; i < line.length; i++) {
    const char = line[i];

    if (inQuotes) {
      if (char === '"') {
        if (line[i + 1] === '"') {
          current += '"';
          i++;
        } else {
          inQuotes = false;
        }
      } else {
        current += char;
      }
    } else if (char === '"') {
      inQuotes = true;
    } else if (char === ',') {
      cells.push(current.trim());
      current = '';
    } else {
      current += char;
    }
  }
  cells.push(current.trim());
  return cells;
};

export const parseUserInviteCsv = (text: string): BulkInviteCsvRow[] => {
  const lines = text
    .split(/\r\n|\n/)
    .map((line) => line.trim())
    .filter((line) => line.length > 0);

  if (lines.length < 2) return [];

  const headers = parseCsvLine(lines[0]).map((h) => h.toLowerCase());

  return lines
    .slice(1)
    .map((line) => {
      const cells = parseCsvLine(line);
      const row: Record<string, string> = {};
      headers.forEach((header, idx) => {
        row[header] = cells[idx] ?? '';
      });
      return {
        email: row.email || '',
        first_name: row.first_name || '',
        last_name: row.last_name || '',
        role: row.role || '',
        password: row.password || '',
        confirm_password: row.confirm_password || '',
      };
    })
    .filter((row) => row.email.length > 0);
};
