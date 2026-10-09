import type { StaticSource } from 'fumadocs-core/source';
import { loader } from 'fumadocs-core/source';
import { type CollectionEntry, getCollection } from 'astro:content';
import * as path from 'node:path';
import { structure, type StructuredData } from 'fumadocs-core/mdx-plugins';
import type { Node, Root } from 'fumadocs-core/page-tree';

export const source = loader({
  source: await createMySource(),
  baseUrl: import.meta.env.BASE_URL,
});

export function getStructuredData(entry: CollectionEntry<'docs'>): StructuredData {
  return structure(entry.body);
}

export function getPageTree(): Root {
  const tree = source.getPageTree();
  return { ...tree, children: tree.children.map(withBaseUrl) };
}

function withBaseUrl(node: Node): Node {
  if (node.type === 'page') {
    return { ...node, url: prefixBase(node.url) };
  }

  if (node.type === 'folder') {
    return {
      ...node,
      index: node.index ? { ...node.index, url: prefixBase(node.index.url) } : undefined,
      children: node.children.map(withBaseUrl),
    };
  }

  return node;
}

function prefixBase(url: string) {
  if (!url.startsWith('/') || url.startsWith('//')) return url;

  const base = import.meta.env.BASE_URL.replace(/\/$/, '');
  if (!base || url === base || url.startsWith(`${base}/`)) return url;
  return `${base}${url}`;
}

async function createMySource() {
  const out: StaticSource<{
    metaData: CollectionEntry<'meta'>['data'];
    pageData: CollectionEntry<'docs'>['data'] & {
      _raw: CollectionEntry<'docs'>;
    };
  }> = {
    files: [],
  };

  for (const page of await getCollection('docs')) {
    const virtualPath = path.relative('content/docs', page.filePath!);

    out.files.push({
      type: 'page',
      path: virtualPath,
      data: {
        ...page.data,
        _raw: page,
      },
    });
  }

  for (const meta of await getCollection('meta')) {
    const virtualPath = path.relative('content/docs', meta.filePath!);

    out.files.push({
      type: 'meta',
      path: virtualPath,
      data: meta.data,
    });
  }

  return out;
}
