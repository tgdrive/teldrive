import { mkdir, copyFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';

const source = resolve(import.meta.dir, '../../openapi/teldrive.openapi.yaml');
const destination = resolve(import.meta.dir, '../public/openapi.yaml');

await mkdir(dirname(destination), { recursive: true });
await copyFile(source, destination);
console.log(`synced ${source} -> ${destination}`);
