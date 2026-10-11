export function makeLargePdf(pageCount = 240) {
  const objects = new Map<number, string>();
  const pageIds = Array.from({ length: pageCount }, (_, index) => 6 + index * 2);
  objects.set(1, "<< /Type /Catalog /Pages 2 0 R /Outlines 3 0 R >>");
  objects.set(
    2,
    `<< /Type /Pages /Count ${pageCount} /Kids [${pageIds.map((id) => `${id} 0 R`).join(" ")}] >>`,
  );
  objects.set(3, "<< /Type /Outlines /First 4 0 R /Last 4 0 R /Count 1 >>");
  objects.set(
    4,
    `<< /Title (Middle chapter) /Parent 3 0 R /Dest [${pageIds[Math.floor(pageCount / 2)]} 0 R /Fit] >>`,
  );
  objects.set(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>");
  for (const [index, id] of pageIds.entries()) {
    objects.set(
      id,
      `<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents ${id + 1} 0 R >>`,
    );
    const text = Array.from(
      { length: 35 },
      (_, line) =>
        `BT /F1 12 Tf 48 ${740 - line * 18} Td (Page ${index + 1} - line ${line + 1}: PDF reader performance fixture.) Tj ET`,
    ).join("\n");
    objects.set(id + 1, `<< /Length ${text.length} >>\nstream\n${text}\nendstream`);
  }
  let result = "%PDF-1.4\n";
  const offsets = [0];
  for (const [id, object] of objects) {
    offsets[id] = result.length;
    result += `${id} 0 obj\n${object}\nendobj\n`;
  }
  const xref = result.length;
  result += `xref\n0 ${offsets.length}\n0000000000 65535 f \n`;
  result += offsets
    .slice(1)
    .map((offset) => `${String(offset).padStart(10, "0")} 00000 n \n`)
    .join("");
  result += `trailer\n<< /Size ${offsets.length} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return Buffer.from(result);
}
