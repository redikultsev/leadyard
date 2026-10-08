# Working on leadyard

Rules for people and agents contributing to this repository.

- Language: English for code, docs, commits.
- Read `docs/design.md` before changing behavior; record new decisions in `docs/decisions.md`
  as a new numbered entry. Accepted entries are not edited.
- Evidence over claims: a PR states how it was verified, with links or command output.
- A change to a skill needs a before/after measurement (design §17).
- Keep diffs minimal: no flags, env vars, files or dependencies that the task did not ask for.
- Commits: Conventional Commits; trailer `Assisted-by: <agent>/<model>` when an agent helped;
  no other co-author trailers.
- Never commit secrets. Config holds names of environment variables, not values.
