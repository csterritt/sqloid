---
name: Web Behavior
description: Since this is a web development project, here are rules on how to write web code.
---

- use the 'value' attribute for form inputs in edit forms (or wherever a default value is needed), not 'defaultValue'
- When writing a request handler, always use the `redirectWithMessage` or `redirectWithError` functions from `src/lib/redirects.tsx` to redirect with a message or error cookie. Only return just text where specifically called out with a comment.
- use data-testid attributes to identify elements for testing
- use kebab-case for data-testid attributes
- ALWAYS name data-testid for either links, buttons, or form submit with 'name-action', not 'name-link', 'name-button', or 'name-submit'
- Always generate proper accessibility: ARIA roles, semantic HTML, and keyboard navigation.
- DaisyUI no longer has the `form-control` class. Use `flex flex-col` instead.
