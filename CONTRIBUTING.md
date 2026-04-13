## New contributor guide

To get an overview of the project, read the [README](README.md) file. Here are some resources to help you get started with open source contributions:

- [Finding ways to contribute to open source on GitHub](https://docs.github.com/en/get-started/exploring-projects-on-github/finding-ways-to-contribute-to-open-source-on-github)
- [Set up Git](https://docs.github.com/en/get-started/quickstart/set-up-git)
- [GitHub flow](https://docs.github.com/en/get-started/quickstart/github-flow)
- [Collaborating with pull requests](https://docs.github.com/en/github/collaborating-with-pull-requests)

## Development Setup

1. Clone the repository.
2. Start all services in development mode:
   ```bash
   cd deploy/compose && ./run_debug.sh
   ```
   This starts the backend services and the frontend with hot-reload.
3. To stop services:
   ```bash
   cd deploy/compose && ./stop.sh
   ```

## Build Rules

All builds and tests must run through Docker. Never use host tools (`npx`, `npm`, `node`, `go`) directly. If your user needs the docker group, use `sg docker -c "..."`.

**Verify frontend changes:**
```bash
sg docker -c "cd deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build frontend"
```

**Run tests:**
```bash
cd deploy/compose && docker compose -f docker-compose.test.yaml --profile unit run --rm <service>
```

## Code Style

- **Go backend:** Standard Go conventions. Run `go vet` and `go fmt`.
- **Frontend:** Next.js 15 with Material UI 6. Follow existing patterns in `src/pages/`, `src/sections/`, and `src/contexts/`.
- **Commit messages:** Descriptive, imperative mood (e.g., "Add tenant deletion endpoint", "Fix offline device cache").

## Multi-Tenancy

Oktopus is a multi-tenant platform. When writing new API handlers:

- Use `a.tenantDB(r)` for all tenant-scoped data access. Never use `a.db` directly for tenant data.
- Tenant-scoped routes live under `/api/tenants/{slug}/` and pass through both `AuthMiddleware` and `TenantMiddleware`.
- NATS subjects must include the tenant slug for proper routing isolation.

## Architecture Changes

When modifying project architecture, conventions, dependencies, build commands, or infrastructure, update the `CLAUDE.md` file to keep it accurate.

## Pull Request

When you're finished with the changes, create a pull request, also known as a PR.
- Set your PR to go into the **main branch**.
- Don't forget to [link PR to issue](https://docs.github.com/en/issues/tracking-your-work-with-issues/linking-a-pull-request-to-an-issue) if you are solving one.
- Enable the checkbox to [allow maintainer edits](https://docs.github.com/en/github/collaborating-with-issues-and-pull-requests/allowing-changes-to-a-pull-request-branch-created-from-a-fork) so the branch can be updated for a merge.
Once you submit your PR, a team member will review your proposal. We may ask questions or request additional information.
- We may ask for changes to be made before a PR can be merged, either using [suggested changes](https://docs.github.com/en/github/collaborating-with-issues-and-pull-requests/incorporating-feedback-in-your-pull-request) or pull request comments. You can apply suggested changes directly through the UI. You can make any other changes in your fork, then commit them to your branch.
- As you update your PR and apply changes, mark each conversation as [resolved](https://docs.github.com/en/github/collaborating-with-issues-and-pull-requests/commenting-on-a-pull-request#resolving-conversations).
- If you run into any merge issues, checkout this [git tutorial](https://github.com/skills/resolve-merge-conflicts) to help you resolve merge conflicts and other issues.
