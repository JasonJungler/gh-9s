// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLlmsTxt from 'starlight-llms-txt';

// https://astro.build/config
export default defineConfig({
	site: 'https://murabitob.github.io',
	base: '/gh-9s',
	integrations: [
		starlight({
			title: 'gh-9s',
			description: 'A k9s-style terminal UI for GitHub, as a gh extension.',
			customCss: ['./src/styles/custom.css'],
			plugins: [
				starlightLlmsTxt({
					projectName: 'gh-9s',
					description: 'A k9s-style terminal UI for GitHub, as a gh extension. Covers Actions runs and job graphs, pull requests and reviews, issues, the inbox, repositories, GitHub Projects, and settings.',
					optionalLinks: [
						{ label: 'Source code', url: 'https://github.com/MurabitoB/gh-9s', description: 'The Go source and release history' },
					],
					customSets: [
						{ label: 'Reference', paths: ['reference/**'], description: 'Commands, keys, and files and environment variables' },
					],
				}),
			],
			social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/MurabitoB/gh-9s' }],
			sidebar: [
				{ label: 'Getting started', slug: 'guides/getting-started' },
				{
					label: 'Using gh-9s',
					items: [
						{ label: 'Interface basics', slug: 'guides/interface' },
						{ label: 'Runs and logs', slug: 'guides/runs-and-logs' },
						{ label: 'Job graph', slug: 'guides/job-graph' },
						{ label: 'Workflows and favorites', slug: 'guides/workflows' },
						{ label: 'Pull requests', slug: 'guides/pull-requests' },
						{ label: 'Reviewing a PR', slug: 'guides/reviewing-prs' },
						{ label: 'Issues', slug: 'guides/issues' },
						{ label: 'Send an issue to an agent', slug: 'guides/send-to-agent' },
						{ label: 'Discussions', slug: 'guides/discussions' },
						{ label: 'Wiki', slug: 'guides/wiki' },
						{ label: 'Inbox', slug: 'guides/inbox' },
						{ label: 'Repositories', slug: 'guides/repos' },
						{ label: 'Project boards', slug: 'guides/projects' },
						{ label: 'Images and Markdown', slug: 'guides/images-and-markdown' },
						{ label: 'Settings', slug: 'guides/settings' },
					],
				},
				{
					label: 'Reference',
					items: [
						{ label: 'Commands', slug: 'reference/commands' },
						{ label: 'Keys', slug: 'reference/keys' },
						{ label: 'Files and environment', slug: 'reference/files-and-env' },
					],
				},
			],
		}),
	],
});
