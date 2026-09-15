import type { ComponentProps } from 'react';
import defaultMdxComponents from 'fumadocs-ui/mdx';

function withBase(href: string | undefined) {
  if (!href || !href.startsWith('/') || href.startsWith('//')) return href;

  const base = import.meta.env.BASE_URL.replace(/\/$/, '');
  return `${base}${href}`;
}

const DefaultLink = defaultMdxComponents.a;
const DefaultCard = defaultMdxComponents.Card;

function BaseAwareLink(props: ComponentProps<typeof DefaultLink>) {
  return <DefaultLink {...props} href={withBase(props.href)} />;
}

function BaseAwareCard(props: ComponentProps<typeof DefaultCard>) {
  return <DefaultCard {...props} href={withBase(props.href)} />;
}

export const mdxComponents = {
  ...defaultMdxComponents,
  a: BaseAwareLink,
  Card: BaseAwareCard,
};
