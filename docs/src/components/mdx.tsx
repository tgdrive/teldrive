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

function ButtonLink({ className, ...props }: ComponentProps<'a'>) {
  return (
    <a
      {...props}
      href={withBase(props.href)}
      className={`not-prose inline-flex items-center rounded-lg bg-fd-primary px-4 py-2.5 font-medium text-fd-primary-foreground no-underline hover:opacity-90 ${className ?? ''}`}
    />
  );
}

export const mdxComponents = {
  ...defaultMdxComponents,
  a: BaseAwareLink,
  Card: BaseAwareCard,
  ButtonLink,
};
