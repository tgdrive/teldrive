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
  ConfigOption,
};

function ConfigOption({ name, description, defaultValue, flag, env }: {
  name: string; description: string; defaultValue: string; flag: string; env: string;
}) {
  return (
    <section className="config-option not-prose" aria-label={name}>
      <h3><code>{name}</code></h3>
      <p>{description}</p>
      <dl>
        <div><dt>Default</dt><dd><code>{defaultValue || 'Not set'}</code></dd></div>
        <div><dt>CLI flag</dt><dd><code>{flag}</code></dd></div>
        <div><dt>Environment</dt><dd><code>{env}</code></dd></div>
      </dl>
    </section>
  );
}
