; Go symbol extraction queries
(function_declaration
  name: (identifier) @symbol.function)

(method_declaration
  name: (field_identifier) @symbol.method)

(type_declaration
  (type_spec
    name: (type_identifier) @symbol.type))
