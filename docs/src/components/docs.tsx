import { DocsLayout } from 'fumadocs-ui/layouts/docs';
import { DocsPage, type DocsPageProps } from 'fumadocs-ui/layouts/docs/page';
import type { Root } from 'fumadocs-core/page-tree';
import type { ReactNode } from 'react';
import { navigate } from 'astro:transitions/client';
import { RootProvider } from 'fumadocs-ui/provider/astro';
import type { AstroProviderProps } from 'fumadocs-core/framework/astro';
import SearchDialog from './search';

export function Docs({
  tree,
  children,
  pathname,
  params,
  page,
}: {
  tree: Root;
  children: ReactNode;
  pathname: string;
  params: AstroProviderProps['params'];
  page?: DocsPageProps;
}) {
  return (
    <RootProvider
      pathname={pathname}
      params={params}
      navigate={navigate}
      theme={{ enabled: true, attribute: 'class', defaultTheme: 'system', enableSystem: true }}
      search={{ SearchDialog }}
    >
      <DocsLayout
        tree={tree}
        githubUrl="https://github.com/tgdrive/teldrive"
        themeSwitch={{ enabled: true }}
        nav={{
          title: 'Teldrive',
          url: import.meta.env.BASE_URL,
        }}
        sidebar={{
          defaultOpenLevel: 1,
        }}
      >
        <DocsPage {...page}>{children}</DocsPage>
      </DocsLayout>
    </RootProvider>
  );
}
