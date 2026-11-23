# The Algebraic Data Model

There's widespread consensus in the database community that the relational
model (RM) is sound.  In
["What Goes Around Comes Around... And Around..."](https://db.cs.cmu.edu/papers/2024/whatgoesaround-sigmodrec2024.pdf), Pavlo and Stonebraker argue
that any attempt to replace it will fail, because whatever good
ideas such efforts produce will simply be absorbed into SQL and the RM.
Prof. Pavlo in fact teaches his students that
"in the same way ... one plus one is two ..., the relational model is the true
way, the correct way ... to represent data"
([lecture](https://www.youtube.com/watch?t=4466&v=APqWIjtzNGE)).
If only the [anachronisms of SQL](https://www.scattered-thoughts.net/writing/against-sql/)
could be fixed, e.g., with
[Pipe SQL](https://research.google/pubs/sql-has-problems-we-can-fix-them-pipe-syntax-in-sql/)
or [PRQL](https://prql-lang.org/), then everything would be perfect.

We don't agree, and SuperDB takes a very different view.
Better syntax keeps the RM's type system, and with it, the RM's trouble with
data that doesn't fit in a table.  The relational _operators_ are great:
selection, projection, joins, aggregation, normalized tables, and primary keys
have stood the test of time.  The problem is that they are bound to tables,
which are rooted in mathematical relations, i.e., sets of tuples that all
have the same type.  Real-world data is rarely so uniform.

The _algebraic data model_ (ADM) keeps the relational operators but frees them
from tables.  It is based on the algebraic types of modern programming
languages: records, arrays, sets, maps, and, crucially, _sum types_, also known
as unions or tagged unions.  A relational table is just a special case,
i.e., a collection of records that all have the same type.  When the relational
operators are applied to algebraic types, they become simpler and more
natural to use, as the rest of this section shows.

## A Running Example

Here is some data describing shapes, which we will use throughout this section:
```mdtest-input shapes.json
{"id":1,"type":"Rectangle","dimensions":{"width":2,"height":1}}
{"id":2,"type":"Circle","dimensions":{"radius":3}}
{"id":3,"type":"Square","dimensions":{"side":5}}
{"id":4,"type":"Circle","dimensions":{"radius":7}}
{"id":5,"type":"Vector","dimensions":[1,2,3,2,1]}
```
The `dimensions` of a shape depend on what kind of shape it is.
In a programming language, you would model this with a sum type, e.g.,
in Rust,
```rust
enum Dimensions {
    Rectangle { width: i64, height: i64 },
    Circle { radius: i64 },
    Square { side: i64 },
    Vector(Vec<i64>),
}
```

## Relations Are Product Types

In the RM, a relation is a set of tuples over a fixed list of columns, i.e.,
a set of values of a single _product type_.  Codd's original paper on the
relational model describes a relation in just these terms.

>[!NOTE]
> Codd's original paper on the relational model has a footnote that
> essentially describes a relation as a product type:
>
> <!-- markdownlint-disable-next-line MD033 -->
> <center><img src="codd-footnote.png" alt="Codd's Footnote" title="Codd's Footnote" width="500"></center>
>
> But sum types were notably absent.

Without sum types, the shapes don't fit, so a relational schema has to fake
the sum type with one of these workarounds:
* a _wide table_ with a nullable column for every field of every kind of shape,
  e.g., `width`, `height`, `radius`, `side`, and `vector`, where most values in
  each row are `NULL`,
* a _normalized_ table for each kind of shape, which must be
  recombined with `UNION ALL` or outer joins whenever the shapes are queried
  together, again padding the results with `NULL`, or
* a _JSON column_, which gives up on types altogether and requires runtime
  checks in every query, as described in
  [JSON Can't Tell a Map from a Record](blog/json-map-or-record.md).

In each case, `NULL` ends up standing for "this field doesn't apply to this
kind of shape," which SQL can't tell apart from "this value is unknown."

## Shapes in the ADM

With algebraic types, the shapes need no workaround.  Each shape is a record
with its own precise type, and saving the shapes as [BSUP](formats/bsup.md)
keeps these types with the data:
```mdtest-command
super -o shapes.bsup shapes.json
super -s -c 'from shapes.bsup | values typeof(this)'
```
```mdtest-output
<{id:int64,type:string,dimensions:{width:int64,height:int64}}>
<{id:int64,type:string,dimensions:{radius:int64}}>
<{id:int64,type:string,dimensions:{side:int64}}>
<{id:int64,type:string,dimensions:{radius:int64}}>
<{id:int64,type:string,dimensions:[int64]}>
```
Under the covers, SuperDB [fuses](super-sql/type-fusion.md) these types into
one type that describes the whole collection, which the type checker uses to
check queries:
```mdtest-command
super -s -c 'from shapes.bsup | aggregate fuse(this)'
```
```mdtest-output
<{id:int64,type:string,dimensions:{width:int64|missing,height:int64|missing,radius:int64|missing,side:int64|missing}|[int64]}>
```
Here `dimensions` is a sum type: either a record or an array of integers.
And a field that only some shapes have is marked `missing` rather than
filled with `NULL`, so "doesn't apply" and "unknown" are never confused.

## The Relational Operators, Generalized

### Join Is Record Composition

In SuperSQL, a [join](super-sql/operators/join.md) doesn't flatten its inputs
into one wide row.  It composes them: each result is a record with a `left`
field and a `right` field holding the two values that matched.
Given some paint colors for the shapes:
```mdtest-input paint.json
{"id":1,"color":"red"}
{"id":2,"color":"blue"}
{"id":4,"color":"green"}
```
the join is
```mdtest-command
super -s -c 'from shapes.bsup | join (from paint.json) on left.id=right.id'
```
```mdtest-output
{left:{id:1,type:"Rectangle",dimensions:{width:2,height:1}},right:{id:1,color:"red"}}
{left:{id:2,type:"Circle",dimensions:{radius:3}},right:{id:2,color:"blue"}}
{left:{id:4,type:"Circle",dimensions:{radius:7}},right:{id:4,color:"green"}}
```
and the matched values can be combined in any way you like, e.g.,
by spreading a shape into a new record along with its color:
```mdtest-command
super -s -c 'from shapes.bsup | join (from paint.json) on left.id=right.id | values {...left, color:right.color}'
```
```mdtest-output
{id:1,type:"Rectangle",dimensions:{width:2,height:1},color:"red"}
{id:2,type:"Circle",dimensions:{radius:3},color:"blue"}
{id:4,type:"Circle",dimensions:{radius:7},color:"green"}
```

An outer join shows the difference most clearly.  In SQL, a `LEFT JOIN`
pads an unmatched row with `NULL` columns, which look just like a color that
is actually `NULL`.  In SuperSQL, an unmatched shape simply has no `right`
field, and the query says what to do about it:
```mdtest-command
super -s -c "from shapes.bsup | left join (from paint.json) on left.id=right.id | values {...left, color:right.color.ok() ?? 'unpainted'} | sort id"
```
```mdtest-output
{id:1,type:"Rectangle",dimensions:{width:2,height:1},color:"red"}
{id:2,type:"Circle",dimensions:{radius:3},color:"blue"}
{id:3,type:"Square",dimensions:{side:5},color:"unpainted"}
{id:4,type:"Circle",dimensions:{radius:7},color:"green"}
{id:5,type:"Vector",dimensions:[1,2,3,2,1],color:"unpainted"}
```

Keys and normalization work just as they do in the RM.  A
[table](super-sql/statements/create-table.md) can have typed columns and
`NOT NULL` constraints, and tables are joined on their keys.  The ADM
simply doesn't require every value in a table to have the same type.

### Unnest Instead of Lateral Joins

To work with the elements of the vector in SQL, you would unnest the array
with a lateral join, which joins each row with a table computed from that row:
```sql
SELECT s.id, v.d
FROM shapes s CROSS JOIN LATERAL unnest(s.vector) AS v(d)
```
In SuperSQL, [unnest](super-sql/operators/unnest.md) is an operator of its own.
A [type assertion](super-sql/expressions/type-assertions.md) picks out the
dimensions that are arrays, the other shapes produce nothing, and
`with` carries along the shape's `id`:
```mdtest-command
super -s -c 'from shapes.bsup | unnest d:=dimensions.([int64]) with id'
```
```mdtest-output
{d:1,id:5}
{d:2,id:5}
{d:3,id:5}
{d:2,id:5}
{d:1,id:5}
```
And with `into`, a query runs on the elements of each array as a unit,
just like a lateral subquery but without the join:
```mdtest-command
super -s -c 'from shapes.bsup | unnest d:=dimensions.([int64]) with id into (aggregate total:=sum(d), peak:=max(d) by id)'
```
```mdtest-output
{id:5,total:9,peak:3}
```

### No NULL Surprises

In SQL, comparing `NULL` to anything yields `NULL`, which `WHERE` treats as
false.  So with the wide table above, the rows returned by
`WHERE radius > 5` and by `WHERE NOT (radius > 5)` don't add up to the
whole table: every shape that isn't a circle silently disappears from both.
And a misspelled JSON path just returns `NULL`, so the query still runs
and gives the wrong answer.

In SuperSQL, the type checker makes you work out what to do about a field
that might be missing before the query runs.  Asking for the radius of every
shape is an error, because the checker knows which shapes don't have one:
```mdtest-command fails
super -s -c 'from shapes.bsup | where dimensions.radius > 5 | values id'
```
```mdtest-output
no such field "radius": . applied to [int64] in {width:int64|missing,height:int64|missing,radius:int64|missing,side:int64|missing}|[int64] at line 1, column 37:
from shapes.bsup | where dimensions.radius > 5 | values id
                                    ~~~~~~
field may be missing (consider ok()) at line 1, column 26:
from shapes.bsup | where dimensions.radius > 5 | values id
                         ~~~~~~~~~~~~~~~~~
```
Saying that the radius may be absent with [ok](super-sql/functions/generics/ok.md)
isn't enough either, as the checker then wants to know what an absent
radius should mean:
```mdtest-command fails
super -s -c 'from shapes.bsup | where dimensions.radius.ok() > 5 | values id'
```
```mdtest-output
int64|none: none not handled (consider ?? or ?.) at line 1, column 26:
from shapes.bsup | where dimensions.radius.ok() > 5 | values id
                         ~~~~~~~~~~~~~~~~~~~~~~
```
Once the query says it, e.g., that a shape without a radius counts as
radius 0, the filter and its negation together account for every shape:
```mdtest-command
super -s -c 'from shapes.bsup | where (dimensions.radius.ok() ?? 0) > 5 | values id'
super -s -c 'from shapes.bsup | where not ((dimensions.radius.ok() ?? 0) > 5) | values id'
```
```mdtest-output
4
1
2
3
5
```
A misspelled field is caught, too:
```mdtest-command fails
super -s -c 'from shapes.bsup | values dimensions.radious.ok()'
```
```mdtest-output
no such field "radious": . applied to {width:int64|missing,height:int64|missing,radius:int64|missing,side:int64|missing}|[int64] at line 1, column 38:
from shapes.bsup | values dimensions.radious.ok()
                                     ~~~~~~~
```
There are no `NULL` surprises because there is no `NULL` standing in for
missing data: a missing field is part of the type, and the query has to say
what to do about it.

## Types, Not Tables

The ADM redefines the foundation of data around algebraic _types_ instead of
_tables_.  A relational table is a special case, so SuperDB supports
[schema-constrained tables](super-sql/statements/create-table.md) and
SQL queries on them, but a table can also be a
[super table](super-sql/statements/create-super-table.md) of any type, e.g.,
a sum type of the kinds of shapes, or have no type at all.

And because the super formats serialize values with their types, a value's
type is the same whether it's in a query, in a file, or in a database table.
JSON, by contrast, loses the types, as
[JSON Can't Tell a Map from a Record](blog/json-map-or-record.md) explains.

## Not Just a Better SQL

Despite SQL's enduring success,
it is widely accepted that there are serious flaws with the language.
Here are some noteworthy arguments that build the case:

* [A Critique of the SQL Database Language](https://dl.acm.org/doi/pdf/10.1145/984549.984551)
a 1983 paper by
[C.J. Date](https://en.wikipedia.org/wiki/Christopher_J._Date),
* [Against SQL](https://www.scattered-thoughts.net/writing/against-sql/)
by [Jamie Brandon](https://www.scattered-thoughts.net/),
* [A Critique of Modern SQL And A Proposal Towards A Simple and Expressive
Query Language](https://www.cidrdb.org/cidr2024/papers/p48-neumann.pdf)
by [Neumann](https://db.in.tum.de/~neumann/)
and [Leis](https://www.cs.cit.tum.de/dis/team/prof-dr-viktor-leis/).

These publications all argue that SQL is so problematic it should
be replaced outright. In contrast, the authors of
[SQL Has Problems. We Can Fix Them: Pipe Syntax In SQL](https://research.google/pubs/sql-has-problems-we-can-fix-them-pipe-syntax-in-sql/) propose that SQL
should be merely improved upon by creating a superset of SQL.
Except for compositional syntax, the paper says SQL is
perfectly reasonable and we should live with its anachronisms (see Section 2.4).

But better syntax doesn't fix the type system, and bolting JSON or variant
types onto the RM adds a second type system with its own operators rather than
fixing the first.  SuperSQL instead keeps backward compatibility in its SQL
clauses while its pipe operators work natively on algebraic types, e.g.,

* array indexing [can be configured](super-sql/declarations/pragmas.md)
  as 1-based in SQL clauses but 0-based in pipe operators,
* column names in SQL clauses are case insensitive while record field references
are case sensitive in pipe operators,
* complex scoping rules for table aliases and column references are required in
relational SQL while binding from names to data in pipe operators is managed
in a uniform and simple way as dereferenced paths on [this](super-sql/intro.md#pipe-scoping),
* the syntactic structure of SQL clauses means all data must conform to a table
whereas pipe operators can emit any data type desired in a varying fashion, and
* [sum types](super-sql/types/union.md) are integral to piped data allowing mix-typed data processing
and results that need not fit in a uniform table.

> [!NOTE]
> Case insensitive column names in SQL clauses are not yet supported.
> ([super#6066](https://github.com/superdb/super/issues/6066))

With this approach, SuperSQL can be adopted for existing use cases
based on SQL while incrementally embracing the pipe model and algebraic types.

_It's time for the relational model to become a special case of
an easier and broader approach for modern data._
