# The Algebraic Data Model

> "In the same way ... one plus one is two ..., the relational model is the
> true way, the correct way ... to represent data"
>
> &mdash; Andy Pavlo, [CMU database lecture](https://www.youtube.com/watch?t=4466&v=APqWIjtzNGE)

This is what Prof. Pavlo teaches his students, and most of the database
community would agree with him.  In
["What Goes Around Comes Around... And Around..."](https://db.cs.cmu.edu/papers/2024/whatgoesaround-sigmodrec2024.pdf),
Pavlo and Stonebraker argue that any attempt to replace the relational model
(RM) will fail, because whatever good ideas such efforts produce will simply
be absorbed into SQL and the RM.  If only the [anachronisms of SQL](https://www.scattered-thoughts.net/writing/against-sql/)
could be fixed, e.g., with
[Pipe SQL](https://research.google/pubs/sql-has-problems-we-can-fix-them-pipe-syntax-in-sql/),
[PRQL](https://prql-lang.org/), or
[SaneQL](https://www.cidrdb.org/cidr2024/papers/p48-neumann.pdf),
then everything would be perfect.

We don't agree, and SuperDB takes a very different view.
While improvements to SQL's syntax certainly wouldn't hurt, they leave the
RM's type system untouched, and the type system is why the RM has trouble
with data that doesn't fit in a table.  The relational _operators_ are great:
selection, projection, joins, aggregation, normalized tables, and primary keys
have stood the test of time.  The problem is that they are bound to tables,
which are rooted in mathematical relations, i.e., sets of tuples that all
have the same type.  Real-world data is rarely so uniform.

The _algebraic data model_ (ADM) keeps the relational operators but frees them
from tables.  It is based on the algebraic types of modern programming
languages: _product types_ like records, where a value has this field _and_
that one, and _sum types_, also known as unions or tagged unions, where a value
is this _or_ that, along with arrays, sets, and maps.

The argument here is that the two data models in wide use today
each took one half of this picture.  A relational schema is a product type,
with no sums.  JSON is a specific sum type, with no products.  A type system
that has both and composes them freely contains each as a special case.
And because it is the same type system that languages like Rust and TypeScript
already use, data can keep its types all the way from an application to
the database and back.

The ADM is a general idea, like the relational model or the document model.
SuperDB's implementation of it is the [super data model](formats/model.md),
and the examples in this section use its types and the syntax of SuperSQL.

When the relational operators are applied to algebraic types, they also become
simpler and more natural to use, as shown below.

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
struct Shape {
    id: i64,
    dimensions: Dimensions,
}

enum Dimensions {
    Rectangle { width: i64, height: i64 },
    Circle { radius: i64 },
    Square { side: i64 },
    Vector(Vec<i64>),
}
```
`Shape` is a product type: a shape has an `id` _and_ `dimensions`.
`Dimensions` is a sum type: it is a rectangle _or_ a circle _or_ a square
_or_ a vector.

## Relations Are Product Types

In the RM, a relation is a set of tuples over a fixed list of columns.
Each tuple has a value for the first column _and_ a value for the second
_and_ so on, so a tuple is a value of a _product type_ and a table's schema
is exactly one such type.  For example,
```sql
CREATE TABLE paint (id BIGINT NOT NULL, color VARCHAR NOT NULL)
```
declares the product type that the super data model writes as the record type
`{id:int64,color:string}`, and every row must be a value of it.

>[!NOTE]
> Codd's original paper on the relational model has a footnote that
> essentially describes a relation as a product type:
>
> <!-- markdownlint-disable-next-line MD033 -->
> <center><img src="codd-footnote.png" alt="Codd's Footnote" title="Codd's Footnote" width="500"></center>
>
> But sum types were notably absent.

The one sum the RM does have is nullability: a nullable column holds a value
of its type _or_ `NULL`, i.e., `string|null` in the super data model.  Modern SQL systems
have also added nested column types, and a few even have a union column type,
but a relation is still a single product type: every row of a table, and of
every query result, has the same columns.

Without sum types in the schema, the shapes don't fit, so a relational schema
has to fake the sum type with one of these workarounds:
* a _wide table_ with a nullable column for every field of every kind of shape,
  e.g., `width`, `height`, `radius`, `side`, and `vector`, where most values in
  each row are `NULL`,
* a _normalized_ table for each kind of shape, which must be
  recombined with `UNION ALL` or outer joins whenever the shapes are queried
  together, again padding the results with `NULL`, or
* a _JSON column_, which gives up on types altogether and requires runtime
  checks in every query, as described in
  [JSON Can't Tell a Map from a Record](blog/json-map-or-record.md).

In each case, `NULL`, the only sum at hand, ends up standing for "this field
doesn't apply to this kind of shape," which SQL can't tell apart from
"this value is unknown."

## JSON Is a Sum Type

The document model starts from the other end.  A JSON value is a number _or_
a string _or_ a boolean _or_ null _or_ an array of JSON values _or_ an object
whose keys map to JSON values.  That is a sum type and, since JSON values
nest, a recursive one.  SuperSQL can state it directly:
```mdtest-input json.spq
type JSON =
  float64 | int64 | bool | string | null
  | [JSON]
  | map{string:JSON}
```
Casting a shape to this type gives exactly what a JSON column or a document
store holds:
```mdtest-command
super -I json.spq -s -c 'from shapes.json | values this::JSON | head 1'
```
```mdtest-output
type JSON=int64|float64|bool|string|null|[JSON]|map{string:JSON}
map{"id":1,"type":"Rectangle","dimensions":map{"width":2,"height":1}::map{string:JSON}}::map{string:JSON}::JSON
```

So "schemaless" isn't quite right.  A collection of JSON documents does have
a schema.  It is this type, and it is the same type for every collection
ever stored.  But a type that every value satisfies says nothing about any
one of them:
* JSON's objects look like records, but with no type saying which fields an
  object has, a map is the only thing an arbitrary object is guaranteed to be.
  The keys `width` and `height` live in the data, not in the type.
* With nothing to check a key against, a query can't be type checked, and
  a misspelled key quietly evaluates to `null` instead of raising an error.
* The program that wrote this data knew that a rectangle has a `width` and a
  `height` and that a shape is one of four kinds.  None of that survives.

[JSON Can't Tell a Map from a Record](blog/json-map-or-record.md) works
through the consequences in detail.

Where the RM has products without sums, JSON has one fixed sum without
products.

## One Type System for Both

Side by side, the two models are mirror images:

| | Type of a collection | What the type says |
|---|---|---|
| Relational table | one product type, e.g., `{id:int64,color:string}` | everything, provided every row is alike |
| JSON collection | one sum type, `JSON` | nothing, but anything fits |
| ADM | any algebraic type | as much as is true of the data |

Each side has spent years borrowing the other's half.  SQL databases added
JSON and variant columns, and document databases added schema validation.
But in both cases the borrowed half sits outside the native type system:
a JSON column is opaque to the SQL type checker, and a JSON Schema is a
validator run over the data rather than a type the query language understands.

In the ADM, products and sums belong to one type system and compose freely.
A record can have a field that is a union, a union can have members that are
records, an array can hold a union of records, and types can be named and
can refer to themselves.  So
* a relational table is a collection whose type is a record of columns,
* a JSON collection is a collection whose type is `JSON`, and
* everything in between has a type, too.

The shapes live in between.  Their type is neither a flat row nor
"anything goes" but
```
type Rectangle = {width:int64,height:int64}
type Circle = {radius:int64}
type Square = {side:int64}
type Vector = [int64]
type Dimensions = Rectangle|Circle|Square|Vector
type Shape = {id:int64,dimensions:Dimensions}
```
which is the Rust type from above, almost token for token.  Because each
member of `Dimensions` is a [named type](super-sql/types/named.md), the name
says which kind of shape a value is, and the `"type"` string that the JSON
had to carry is no longer needed.

This is what it means for the ADM to subsume the two models.  Neither is
emulated.  A relational schema and the JSON type are both just types that
can be written down, and the relational operators work on all of them.

## Shapes in SuperDB

With algebraic types, the shapes need no workaround, and nobody has to
declare the types above to get started.  When SuperDB reads the JSON, each
shape becomes a record with its own precise type, and saving the shapes as
[BSUP](formats/bsup.md) keeps these types with the data:
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
That holds for the types of a program's own data, too.

## From Application Types to the Database and Back

The types of the ADM weren't invented for databases.  They are the types
application programmers already use.  Rust has structs and enums, TypeScript
has object types and discriminated unions, and Swift, Kotlin, OCaml, Haskell,
and recent versions of Java and Python have their own forms of the same thing.
The shapes in TypeScript, for example, are
```ts
type Shape =
  | { id: number; type: "Rectangle"; dimensions: { width: number; height: number } }
  | { id: number; type: "Circle"; dimensions: { radius: number } }
  | { id: number; type: "Square"; dimensions: { side: number } }
  | { id: number; type: "Vector"; dimensions: number[] };
```

Today these types don't survive the trip to a database:

1. The application serializes its values to JSON.  A struct's fields become
   the keys of a map.  A sum type's tag has nowhere to go, so it is written
   into the data as a string using one of
   [several conventions](https://serde.rs/enum-representations.html), and
   the reader has to know which.  The shapes data above uses one of them,
   with the tag in a `type` field.
2. The database stores the JSON in a JSON column, where its type is `JSON`,
   or shreds it into tables using one of the workarounds above.
3. A query returns rows or more JSON, and the application rebuilds its types
   from them, with a deserializer in Rust or a validation library like
   [Zod](https://zod.dev/) in TypeScript, checking at run time that the data
   still has the shape the program expects.

The types exist at both ends and nowhere in between, and every hop is a
place where the two ends can drift apart.

With the ADM, there is nothing to erase because each of these types has a
direct counterpart, shown here for the super data model:

| Rust | TypeScript | Super data model |
|---|---|---|
| `struct` | object type | [record](super-sql/types/record.md) |
| `enum` with data | discriminated union | [union](super-sql/types/union.md) of [named types](super-sql/types/named.md) |
| `enum` without data | union of string literals | [enum](super-sql/types/enum.md) |
| `Option<T>` | optional property | [option](formats/model.md#29-option) |
| `Vec<T>` | `T[]` | [array](super-sql/types/array.md) |
| `HashSet<T>` | `Set<T>` | [set](super-sql/types/set.md) |
| `HashMap<K, V>` | `Map<K, V>` | [map](super-sql/types/map.md) |

And because the [super formats](formats/intro.md) carry types along with
values, a type goes wherever its values go:
* a `Shape` serialized by the application arrives at the database as a `Shape`,
* a [super table](super-sql/statements/create-super-table.md) declared with
  type `Shape` holds only values of that type,
* queries on the table are type checked against it, and
* query results deserialize back into a `Shape`, whether in the Rust service
  that wrote it or the TypeScript client that displays it.

There is no mapping layer to configure and no second copy of the schema to
keep in sync.  One type, declared once in the program, is the type on the
wire, in the file, in the table, and in the query.

> [!NOTE]
> Serialization of native types is available today in the
> [Go library](dev/libraries/go.md).  Libraries that do the same for Rust and
> TypeScript are under development.

## Earlier Attempts to Close the Gap

The gap between application types and database types is known as the
[object-relational impedance mismatch](https://en.wikipedia.org/wiki/Object%E2%80%93relational_impedance_mismatch),
and the ADM is far from the first attempt to close it.  The earlier attempts
are instructive because each one left the two type systems different and
tried to connect them from the outside.

**Object-relational mappers** keep the RM in the database and translate
between objects and rows in a library.  ORMs are everywhere, and so are
complaints about them, because the translation never quite disappears.
The trouble starts exactly where sum types do.  To store a class hierarchy,
an ORM offers
[single-table inheritance](https://martinfowler.com/eaaCatalog/singleTableInheritance.html),
[class-table inheritance](https://martinfowler.com/eaaCatalog/classTableInheritance.html),
or a [serialized LOB](https://martinfowler.com/eaaCatalog/serializedLOB.html),
which are the wide table, the normalized tables, and the JSON column from
[above](#relations-are-product-types).  The ORM automates the workarounds,
but the sum type is still missing.

**Object databases** of the late 1980s and 1990s went the other way and made
the programming language's objects persistent.  That closed the gap for one
language at a time: a database of C++ or Smalltalk objects, with their
pointers, identity, methods, and inheritance, is of little use to a program
written in anything else.  Queries meant navigating from object to object
rather than declaring a result, no standard took hold, and SQL was left
behind along with every tool built on it.

**XML databases** around 2000 offered a third model, a tree of elements and
attributes with its own schema language and its own query language.  It
matched neither side: it isn't how programs structure their data and it
isn't relational.  SQL absorbed XML as a column type, JSON then displaced
XML, and SQL absorbed that, too.

Pavlo and Stonebraker recount the last two episodes in
[their paper](https://db.cs.cmu.edu/papers/2024/whatgoesaround-sigmodrec2024.pdf)
as evidence that challengers to the RM fade away.  But each of these
approaches asked the world to move: to accept a mapping layer, to commit to
one language's object model, or to give up SQL.  The ADM asks for none of
that:
* There is nothing to map, because the database's types and the application's
  types are the same types.
* The types describe values, not objects.  Records, unions, and arrays have
  no identity, methods, pointers, or inheritance, so they mean the same thing
  in every language and in every format.
* Nothing is left behind.  A relational table is a special case, so SQL and
  schema-constrained tables keep working, and JSON is a special case, so
  existing data loads as is.  Algebraic types can be adopted one table,
  or one column, at a time.
* The languages have already arrived.  In the 1990s, mainstream languages had
  classes and little else, so a database had to choose between tables and
  objects.  Today sum types are standard equipment, and the ADM gives
  programmers the types they already write.

The ADM is not a challenger to the RM in the paper's sense.  It is a
generalization that contains it.

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
