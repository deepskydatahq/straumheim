---
name: testing
description: Use when writing or reviewing tests - enforces Kent C. Dodds patterns with convex-test and RTL
---

# Testing Skill

## Testing Trophy (what to test where)

| Layer | Tool | When to use |
|-------|------|-------------|
| **Convex functions** | `convex-test` | Business logic, auth, data validation |
| **React components** | RTL + vi.mock | UI rendering, user interactions |
| **Pure functions** | Vitest only | Algorithms, transformations (validation.ts) |
| **Critical path** | Playwright | Login → core workflow → success state |

## Query Priority (always try in this order)

1. `getByRole` - buttons, links, headings, textboxes
2. `getByLabelText` - form fields
3. `getByText` - non-interactive content
4. `getByTestId` - last resort only

## Test Structure

ALWAYS use setup functions, NEVER beforeEach with shared state:

```typescript
function setup(overrides = {}) {
  const user = userEvent.setup();
  const defaultProps = { productName: "TestApp" };
  render(<Component {...defaultProps} {...overrides} />);

  return {
    user,
    getSubmitButton: () => screen.getByRole("button", { name: /submit/i }),
  };
}
```

## Write Workflow Tests

Combine related assertions into complete user journeys:

```typescript
// ✅ Good: tests what user experiences
test("user completes onboarding flow", async () => {
  const { user, getSubmitButton } = setup();

  expect(screen.getByText(/welcome/i)).toBeInTheDocument();
  await user.type(screen.getByLabelText(/name/i), "Acme");
  await user.click(getSubmitButton());

  expect(await screen.findByText(/success/i)).toBeInTheDocument();
});

// ❌ Bad: tests implementation details
test("sets name state when typing", () => { ... });
```

## Convex Function Tests

```typescript
import { convexTest } from "convex-test";
import schema from "./schema";
import { api } from "./_generated/api";

// Helper to set up authenticated user
async function setupJourney(t: any) {
  const userId = await t.run(async (ctx) => {
    return await ctx.db.insert("users", {
      clerkId: "test-user",
      email: "test@example.com",
      createdAt: Date.now(),
    });
  });

  const asUser = t.withIdentity({
    subject: "test-user",
    issuer: "https://clerk.test",
    tokenIdentifier: "https://clerk.test|test-user",
  });

  return { userId, asUser };
}

test("mutation validates and persists", async () => {
  const t = convexTest(schema);
  const { asUser } = await setupJourney(t);

  const id = await asUser.mutation(api.journeys.create, {
    type: "overview",
    name: "Test Journey",
  });

  const result = await asUser.query(api.journeys.get, { id });
  expect(result?.name).toBe("Test Journey");
});
```

## Checklist Before Committing Tests

- [ ] Used `getByRole` as primary query
- [ ] Used `userEvent.setup()` not `fireEvent`
- [ ] No `beforeEach` with shared mutable state
- [ ] Tests complete workflows, not single assertions
- [ ] Convex logic tested with `convex-test`, not mocked hooks
