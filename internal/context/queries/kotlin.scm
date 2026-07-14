; Kotlin symbol extraction queries
(function_declaration
  (simple_identifier) @symbol.function)

(class_declaration
  (type_identifier) @symbol.class)

(object_declaration
  (type_identifier) @symbol.class)
