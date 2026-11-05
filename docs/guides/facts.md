# Facts

**Facts** are the things that are true about your project: your business name, phone number, prices, opening hours, delivery area, and house rules like "all text in British English".

They're how RaisQ catches an AI making things up. When the agent writes "Call us on 02-999-0000", the critic checks it against your facts. If your real number is 02-123-4567, that attempt fails.

## Do I need facts?

If your project shows information to people, such as a website, docs or emails, yes. Facts are what let RaisQ check that information.

If it's a library or a backend with nothing a user reads, you can skip them. RaisQ still runs your tests and checks the task's criteria.

## Adding facts

When you set up a project from chat, your agent asks you for the facts it finds the project shows people. To add more:

```
raisq fact add business/phone "02-123-4567" --kind contact
raisq fact add business/hours "Tuesday to Sunday, 8:00 to 18:00" --kind hours
raisq fact add orders/notice "Cake orders need 2 days' notice"
```

The first part, such as `business/phone`, is a name you choose. The part before the `/` groups related facts together.

The kind tells RaisQ how often a fact needs rechecking:

| Kind | For | Recheck every |
|---|---|---|
| `contact` | Phone, email, address | 180 days |
| `hours` | Opening hours | 90 days |
| `price` | Prices | 30 days |
| `terms` | Delivery, returns, policies | 90 days |
| `rule` | House rules, like "no exclamation marks" | never |

When a fact is past its recheck date, RaisQ treats it as unconfirmed until you confirm it again. `raisq status` lists the ones that are due.

## How sure are you?

Every fact says how you know it:

| Tag | Means | What RaisQ does with it |
|---|---|---|
| **confirmed** | A person told you, and you're sure | A claim that matches it passes |
| **inferred** | A guess, or something nobody has checked | A claim based on it is shown to you as *unsure* |
| **gap** | You know you don't know it | The agent is told to leave it out, and a claim about it fails |

`raisq fact add` marks a fact as confirmed by you, recorded with your name from `git config user.name`. Facts are committed with the project, so your teammates' tasks are checked against them too. If someone else, such as a client, needs to confirm it:

```
raisq fact add orders/minimum "Minimum order 500 baht" --needs-stakeholder
```

It stays *inferred* until it's confirmed. When the client has confirmed it, record who did:

```
raisq fact confirm orders/minimum --by "Mai (owner)"
```

A **gap** is useful. If you don't know your minimum order yet, add it as a gap, and the agent won't invent one:

```
raisq fact add orders/minimum --gap
```

## Changing a fact

Use `raisq fact add` again with the same name:

```
raisq fact add business/phone "02-555-1234"
```

If a task that's waiting for review relied on the old number, RaisQ sends it back to the queue, so it's rebuilt with the right one. Tasks that didn't use it aren't affected.

## Looking at your facts

```
raisq fact list                  # all of them
raisq fact list business         # one group
raisq fact list --tag inferred   # the ones nobody has confirmed
raisq fact list --due            # the ones past their recheck date
raisq fact list --kind rule      # your house rules
raisq fact list --said-by mai    # the ones a person told you
```

Facts are stored as plain YAML in `.raisq/facts/`, one file per group. `--topic` on `raisq fact add` puts a fact in another file; a fact you change stays in the file it's in. You can edit them by hand, but `raisq fact` keeps track of which tasks relied on what. See [Configuration](../reference/configuration.md#facts) for the file format.
