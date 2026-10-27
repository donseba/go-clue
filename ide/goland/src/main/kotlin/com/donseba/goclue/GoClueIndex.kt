package com.donseba.goclue

import com.google.gson.Gson
import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.LocalFileSystem
import java.io.File
import java.nio.charset.StandardCharsets
import java.util.concurrent.ConcurrentHashMap

data class GoClueType(
    val fqName: String,
    val name: String,
    val pkg: String,
    val file: String,
    val line: Int,
    val column: Int,
    val doc: String,
    val fields: Map<String, GoClueField>,
    val methods: Map<String, GoClueMethod>,
    // underlying is what a named type other than a struct or an interface is
    // defined as, such as "[7]Day" for OpeningHours, relative to its package.
    val underlying: String = "",
)

data class GoClueField(
    val name: String,
    val type: String,
    val doc: String,
    val file: String,
    val line: Int,
    val column: Int,
)

data class GoClueMethod(
    val name: String,
    val type: String,
    val signature: String,
    val doc: String,
    val file: String,
    val line: Int,
    val column: Int,
)

data class GoClueFunc(
    val fqName: String,
    val name: String,
    val pkg: String,
    val file: String,
    val line: Int,
    val column: Int,
    val doc: String,
    val result: String,
    val signature: String,
    val params: List<String>,
)

data class TemplateContract(
    val name: String = "",
    val roots: Map<String, String>,
    val dot: String = "",
    val funcs: Map<String, String> = emptyMap(),
    val gens: Map<String, String> = emptyMap(),
    val source: String = "",
    val line: Int = 0,
    val column: Int = 0,
) {
    fun typedRootType(name: String): String? = roots[name]

    fun isTypedRoot(name: String, typeName: String): Boolean = typedRootType(name) == typeName
}

class GoClueIndex(
    val types: Map<String, GoClueType>,
    val funcs: Map<String, GoClueFunc>,
    val templates: Map<String, TemplateContract>,
    val short: Map<String, List<String>>,
    val symbolAliases: Map<String, String> = emptyMap(),
    val symbolStrictMode: Boolean = false,
    val source: String? = null,
    val rootPath: String? = null,
    val checkedPaths: List<String> = emptyList(),
    val loadError: String? = null,
    // functions are the project's template functions by name, for templates
    // the index does not hold, such as generated ones.
    val functions: Map<String, String> = emptyMap(),
) {
    private fun withCheckedPaths(checkedPaths: List<String>): GoClueIndex = GoClueIndex(
        types = types,
        funcs = funcs,
        templates = templates,
        short = short,
        symbolAliases = symbolAliases,
        symbolStrictMode = symbolStrictMode,
        functions = functions,
        source = source,
        rootPath = rootPath,
        checkedPaths = checkedPaths,
        loadError = loadError,
    )

    companion object {
        fun load(project: Project): GoClueIndex {
            return load(project, null)
        }

        fun load(project: Project, filePath: String?): GoClueIndex {
            val checked = mutableListOf<String>()
            val root = GoClueIndexer.moduleRoot(project, filePath)
            if (root != null && !GoClueIndexer.enabled(project, root)) {
                return empty(checkedPaths = checked)
            }
            val indexFile = if (root != null && GoClueIndexer.autoIndexEnabled(project, root)) {
                findIndexFile(root, checked)
            } else {
                null
            }
                ?: root?.let { shadowIndexFile(it, checked) }
                ?: run {
                    if (root != null) GoClueIndexer.requestIndex(project, root)
                    return empty(checkedPaths = checked)
                }

            return try {
                parseCached(indexFile, root?.path ?: indexFile.parentFile.parentFile.path, checked)
            } catch (err: Throwable) {
                empty(
                    checkedPaths = checked,
                    loadError = "${err.javaClass.simpleName}: ${err.message ?: "failed to load index"}",
                )
            }
        }

        // parsed keeps the last parse of every index file until the file
        // changes. Ctrl+hover asks for the index on each mouse move, and an
        // index can be megabytes of JSON.
        private val parsed = ConcurrentHashMap<String, ParsedIndex>()

        private class ParsedIndex(
            val modified: Long,
            val size: Long,
            val rootPath: String,
            val index: GoClueIndex,
        )

        private fun parseCached(indexFile: File, rootPath: String, checkedPaths: List<String>): GoClueIndex {
            val modified = indexFile.lastModified()
            val size = indexFile.length()
            val cached = parsed[indexFile.path]
            if (cached != null && cached.modified == modified && cached.size == size && cached.rootPath == rootPath) {
                return cached.index.withCheckedPaths(checkedPaths)
            }

            val index = parse(readIndexText(indexFile), indexFile.path, rootPath, checkedPaths)
            parsed[indexFile.path] = ParsedIndex(modified, size, rootPath, index)
            return index
        }

        private fun empty(
            checkedPaths: List<String> = emptyList(),
            loadError: String? = null,
        ): GoClueIndex = GoClueIndex(
            types = emptyMap(),
            funcs = emptyMap(),
            templates = emptyMap(),
            short = emptyMap(),
            symbolAliases = emptyMap(),
            symbolStrictMode = false,
            checkedPaths = checkedPaths,
            loadError = loadError,
        )

        private fun shadowIndexFile(root: File, checked: MutableList<String>): File? {
            val file = GoClueIndexer.shadowIndexFile(root)
            checked.add(file.path)
            return file.takeIf { it.isFile }
        }

        private fun findIndexFile(root: File, checked: MutableList<String>): File? {
            val file = File(root, ".go-clue/index.json")
            checked.add(file.path)
            return file.takeIf { it.isFile }
        }

        private fun readIndexText(file: File): String {
            val bytes = file.readBytes()
            if (bytes.size >= 2) {
                val first = bytes[0].toInt() and 0xff
                val second = bytes[1].toInt() and 0xff
                if (first == 0xff && second == 0xfe) {
                    return String(bytes, StandardCharsets.UTF_16LE)
                }
                if (first == 0xfe && second == 0xff) {
                    return String(bytes, StandardCharsets.UTF_16BE)
                }
            }
            if (bytes.size >= 3) {
                val first = bytes[0].toInt() and 0xff
                val second = bytes[1].toInt() and 0xff
                val third = bytes[2].toInt() and 0xff
                if (first == 0xef && second == 0xbb && third == 0xbf) {
                    return String(bytes.copyOfRange(3, bytes.size), StandardCharsets.UTF_8)
                }
            }

            if (looksLikeUtf16Le(bytes)) {
                return String(bytes, StandardCharsets.UTF_16LE)
            }
            if (looksLikeUtf16Be(bytes)) {
                return String(bytes, StandardCharsets.UTF_16BE)
            }

            return String(bytes, StandardCharsets.UTF_8)
        }

        private fun looksLikeUtf16Le(bytes: ByteArray): Boolean {
            if (bytes.size < 4) return false
            val sample = bytes.take(80)
            val oddZeroes = sample.withIndex().count { (index, value) -> index % 2 == 1 && value.toInt() == 0 }
            return oddZeroes > sample.size / 4
        }

        private fun looksLikeUtf16Be(bytes: ByteArray): Boolean {
            if (bytes.size < 4) return false
            val sample = bytes.take(80)
            val evenZeroes = sample.withIndex().count { (index, value) -> index % 2 == 0 && value.toInt() == 0 }
            return evenZeroes > sample.size / 4
        }

        fun refreshVirtualIndex(project: Project) {
            GoClueIndexer.moduleRoots(project).forEach {
                LocalFileSystem.getInstance().refreshAndFindFileByPath("${it.path}/.go-clue/index.json")
            }
        }

        internal fun parse(json: String, source: String, rootPath: String, checkedPaths: List<String>): GoClueIndex {
            val root = Gson().fromJson(json.trimStart('\uFEFF'), JsonObject::class.java)
            // An index of a version the plugin does not know is not read, and
            // the status and the IDE log say why.
            GoClueIndexVersion.check(root, source)?.let { problem ->
                return GoClueIndex(
                    types = emptyMap(),
                    funcs = emptyMap(),
                    templates = emptyMap(),
                    short = emptyMap(),
                    source = source,
                    rootPath = rootPath,
                    checkedPaths = checkedPaths,
                    loadError = problem,
                )
            }
            val types = mutableMapOf<String, GoClueType>()
            val funcs = mutableMapOf<String, GoClueFunc>()
            val templates = mutableMapOf<String, TemplateContract>()
            val short = mutableMapOf<String, List<String>>()

            jsonObject(root, "types")?.entrySet()?.forEach { (fqName, element) ->
                val obj = element.asJsonObjectOrNull() ?: return@forEach
                val fields = jsonObject(obj, "fields")?.entrySet()?.associate { (name, value) ->
                    if (value.isJsonObject) {
                        val field = value.asJsonObject
                        name to GoClueField(
                            name = name,
                            type = field.get("type")?.asString ?: "",
                            doc = field.get("doc")?.asString ?: "",
                            file = field.get("file")?.asString ?: obj.get("file")?.asString.orEmpty(),
                            line = field.get("line")?.asInt ?: 0,
                            column = field.get("column")?.asInt ?: 0,
                        )
                    } else {
                        name to GoClueField(
                            name = name,
                            type = value.asString,
                            doc = "",
                            file = obj.get("file")?.asString.orEmpty(),
                            line = 0,
                            column = 0,
                        )
                    }
                } ?: emptyMap()
                val methods = jsonObject(obj, "methods")?.entrySet()?.mapNotNull { (name, value) ->
                    val method = value.asJsonObjectOrNull() ?: return@mapNotNull null
                    name to GoClueMethod(
                        name = name,
                        type = method.get("type")?.asString ?: "",
                        signature = method.get("signature")?.asString ?: "",
                        doc = method.get("doc")?.asString ?: "",
                        file = method.get("file")?.asString ?: obj.get("file")?.asString.orEmpty(),
                        line = method.get("line")?.asInt ?: 0,
                        column = method.get("column")?.asInt ?: 0,
                    )
                }?.toMap() ?: emptyMap()
                types[fqName] = GoClueType(
                    fqName = fqName,
                    name = obj.get("name")?.asString ?: fqName.substringAfterLast('.'),
                    pkg = obj.get("package")?.asString ?: "",
                    file = obj.get("file")?.asString ?: "",
                    line = obj.get("line")?.asInt ?: 0,
                    column = obj.get("column")?.asInt ?: 0,
                    doc = obj.get("doc")?.asString ?: "",
                    fields = fields,
                    methods = methods,
                    underlying = obj.get("underlying")?.asString ?: "",
                )
            }

            jsonObject(root, "templates")?.entrySet()?.forEach { (path, element) ->
                val obj = element.asJsonObjectOrNull() ?: return@forEach
                val rootsObject = jsonObject(obj, "roots")
                val roots = rootsObject?.entrySet()?.associate { (name, value) ->
                    name to value.asString
                } ?: emptyMap()
                val funcsObject = jsonObject(obj, "funcs")
                val templateFuncs = funcsObject?.entrySet()?.associate { (name, value) ->
                    name to value.asString
                } ?: emptyMap()
                val gensObject = jsonObject(obj, "gens")
                val gens = gensObject?.entrySet()?.associate { (name, value) ->
                    name to value.asString
                } ?: emptyMap()
                templates[path] = TemplateContract(
                    name = obj.get("name")?.asString ?: "",
                    roots = roots,
                    dot = obj.get("dot")?.asString ?: "",
                    funcs = templateFuncs,
                    gens = gens,
                    source = obj.get("source")?.asString ?: "",
                    line = obj.get("line")?.asInt ?: 0,
                    column = obj.get("column")?.asInt ?: 0,
                )
            }

            jsonObject(root, "funcs")?.entrySet()?.forEach { (fqName, element) ->
                val obj = element.asJsonObjectOrNull() ?: return@forEach
                funcs[fqName] = GoClueFunc(
                    fqName = fqName,
                    name = obj.get("name")?.asString ?: fqName.substringAfterLast('.'),
                    pkg = obj.get("package")?.asString ?: "",
                    file = obj.get("file")?.asString ?: "",
                    line = obj.get("line")?.asInt ?: 0,
                    column = obj.get("column")?.asInt ?: 0,
                    doc = obj.get("doc")?.asString ?: "",
                    result = obj.get("result")?.asString ?: "",
                    signature = obj.get("signature")?.asString ?: "",
                    params = obj.get("params")?.asJsonArray?.mapNotNull { it.asString } ?: emptyList(),
                )
            }

            jsonObject(root, "short")?.entrySet()?.forEach { (name, element) ->
                if (element.isJsonArray) {
                    short[name] = element.asJsonArray.mapNotNull { value -> value.asString }
                }
            }
            val symbolAliases = jsonObject(root, "symbolAliases")?.entrySet()?.associate { (name, value) ->
                name to value.asString
            } ?: emptyMap()
            val symbolStrictMode = root.get("symbolStrictMode")?.asBoolean ?: false
            val functions = jsonObject(root, "functions")?.entrySet()?.associate { (name, value) ->
                name to value.asString
            } ?: emptyMap()

            return GoClueIndex(
                types = types,
                funcs = funcs,
                templates = templates,
                short = short,
                symbolAliases = symbolAliases,
                symbolStrictMode = symbolStrictMode,
                source = source,
                rootPath = rootPath,
                checkedPaths = checkedPaths,
                functions = functions,
            )
        }

        private fun jsonObject(parent: JsonObject, key: String): JsonObject? {
            return parent.get(key)?.asJsonObjectOrNull()
        }

        private fun JsonElement.asJsonObjectOrNull(): JsonObject? {
            return if (isJsonObject) asJsonObject else null
        }
    }

    fun contractForFile(project: Project, filePath: String?): TemplateContract? {
        val path = filePath ?: return null
        val text = templateText(path)
        return contractForFileText(project, path, text, null)
    }

    fun contractForFileAt(project: Project, filePath: String?, offset: Int): TemplateContract? {
        val path = filePath ?: return null
        val text = templateText(path)
        return contractForFileText(project, path, text, offset)
    }

    private fun contractForFileText(project: Project, path: String, text: String?, offset: Int?): TemplateContract? {
        val basePath = rootPath ?: goClueReadAction { project.basePath } ?: return null
        val relative = try {
            File(basePath).toPath().relativize(File(path).toPath()).toString().replace('\\', '/')
        } catch (_: Exception) {
            path.replace('\\', '/')
        }
        val normalizedPath = path.replace('\\', '/')
        val base = templates[relative] ?: templates.entries.firstOrNull { (templatePath, _) ->
            relative.endsWith(templatePath) || normalizedPath.endsWith(templatePath)
        }?.value
        // A template the index does not hold, such as a generated one, still
        // has the project's functions.
        val fileBase = base ?: TemplateContract(roots = emptyMap(), funcs = functions).takeIf { functions.isNotEmpty() }
        val activeBase = offset
            ?.let { activeDefineNameAt(text.orEmpty(), it) }
            ?.let { defineName -> templateByNameInFile(relative, defineName)?.second }
            ?: fileBase
        return mergeInlineContract(path, activeBase)
    }

    fun templateByName(name: String): Pair<String, TemplateContract>? {
        val normalized = name.trimStart('/').replace('\\', '/')
        val entry = templates.entries.firstOrNull { (path, _) ->
            val templatePath = path.replace('\\', '/')
            templatePath == normalized ||
                templatePath.substringAfterLast('/') == normalized ||
                templatePath.endsWith("/$normalized")
        } ?: templates.entries.firstOrNull { (_, contract) ->
            contract.name == normalized
        }
        return entry?.let { it.key to it.value }
    }

    // templateByNameInFile returns the contract of the define name in the
    // template at relative. A define without a contract of its own has none,
    // so the file's contract applies, like in the language server.
    private fun templateByNameInFile(relative: String, name: String): Pair<String, TemplateContract>? {
        val normalized = relative.replace('\\', '/')
        return templates.entries.firstOrNull { (_, contract) ->
            contract.name == name && contract.source.replace('\\', '/') == normalized
        }?.let { it.key to it.value }
    }

    fun fieldsForType(typeName: String?): Map<String, GoClueField> {
        return types[typeName]?.fields.orEmpty()
    }

    fun resolveExpressionType(contract: TemplateContract, expression: String, dotType: String? = null): String? {
        val valueType = resolveExpressionValueType(contract, expression, dotType) ?: return null
        return resolveGoType(valueType)
    }

    fun resolveExpressionValueType(contract: TemplateContract, expression: String, dotType: String? = null): String? {
        val clean = expression.trim()
        if (clean.isBlank()) return null

        if (clean == ".") return dotType
        parenthesizedExpressionPath(clean)?.let { (inner, path) ->
            val rootType = resolveExpressionValueType(contract, inner, dotType) ?: return null
            return resolveFieldValuePath(rootType, path)
        }
        resolveFunctionCommandValueType(contract, clean)?.let { return it }

        val parts = clean.split('.').filter { it.isNotBlank() }
        if (parts.isEmpty()) return null

        val root = parts.first()
        val path: List<String>
        val rootType = when {
            root == "_" -> {
                path = parts.drop(2)
                parts.getOrNull(1)?.let { contract.roots[it] }
            }
            root.startsWith("$") -> {
                path = parts.drop(1)
                contract.roots[root]
            }
            clean.startsWith(".") -> {
                path = parts
                dotType
            }
            else -> {
                path = parts.drop(1)
                contract.typedRootType(root) ?: functionResultValueType(contract, root)
            }
        } ?: return null

        return resolveFieldValuePath(rootType, path)
    }

    private fun resolveFunctionCommandValueType(contract: TemplateContract, expression: String): String? {
        val fields = expression.split(Regex("""\s+""")).filter { it.isNotBlank() }
        if (fields.isEmpty()) return null
        if (fields.size == 1 && fields.first().contains('.')) return null
        return functionResultValueType(contract, fields.first())
    }

    private fun parenthesizedExpressionPath(expression: String): Pair<String, List<String>>? {
        val clean = expression.trim()
        if (!clean.startsWith("(")) return null
        val close = matchingCloseParen(clean, 0)
        if (close < 0 || close + 1 >= clean.length || clean[close + 1] != '.') return null
        val path = clean.substring(close + 1).split('.').filter { it.isNotBlank() }
        if (path.isEmpty()) return null
        return clean.substring(1, close).trim() to path
    }

    private fun matchingCloseParen(text: String, open: Int): Int {
        if (open !in text.indices || text[open] != '(') return -1
        var depth = 0
        var inQuote = false
        var escaped = false
        for (index in open until text.length) {
            val ch = text[index]
            when {
                escaped -> escaped = false
                ch == '\\' -> escaped = true
                ch == '"' -> inQuote = !inQuote
                inQuote -> Unit
                ch == '(' -> depth++
                ch == ')' -> {
                    depth--
                    if (depth == 0) return index
                }
            }
        }
        return -1
    }

    private fun functionResultValueType(contract: TemplateContract, name: String): String? {
        val fnName = contract.funcs[name] ?: return null
        val fn = funcs[fnName] ?: return null
        val result = fn.result.takeIf { it.isNotBlank() } ?: return null
        if (isCompositeValueType(result)) return result
        return resolveGoType(qualifyPackageType(fn.pkg, result)) ?: result
    }

    fun resolveFieldPath(rootType: String, fields: List<String>): String? {
        val valueType = resolveFieldValuePath(rootType, fields) ?: return null
        return resolveGoType(valueType)
    }

    fun resolveFieldValuePath(rootType: String, fields: List<String>): String? {
        var current: String? = rootType
        for ((index, field) in fields.withIndex()) {
            val memberType = memberType(current, field) ?: return null
            if (index == fields.lastIndex) return memberType
            current = resolveGoType(memberType)
        }
        return current
    }

    // memberType is the type of a field or method of ownerType, qualified when
    // it is declared in the owner's own package.
    fun memberType(ownerType: String?, name: String): String? {
        val owner = types[ownerType] ?: return null
        val memberType = owner.fields[name]?.type ?: owner.methods[name]?.type ?: return null
        return qualifyMemberType(owner, memberType)
    }

    // qualifyMemberType qualifies a member type declared in the owner's own
    // package. The index stores those unqualified ("Site", "[]Category",
    // "*Page"), which is ambiguous as soon as another package declares a type
    // with the same name.
    private fun qualifyMemberType(owner: GoClueType, typeExpr: String): String {
        return qualifyPackageType(owner.pkg, typeExpr)
    }

    // qualifyPackageType qualifies a type of package pkg that the index stores
    // unqualified. An index of an older go-clue stores the results of
    // functions like that too.
    private fun qualifyPackageType(pkg: String, typeExpr: String): String {
        val trimmed = typeExpr.trim()
        val base = normalizeGoType(trimmed)
        if (pkg.isBlank() || base.isBlank() || base.any { it in ".[]{}" }) return trimmed
        val qualified = "$pkg.$base"
        if (!types.containsKey(qualified)) return trimmed
        return trimmed.removeSuffix(base) + qualified
    }

    // underlyingType returns what a named type such as OpeningHours ([7]Day)
    // is defined as, qualified against its package, so it can be ranged over.
    // Other types are returned as they are.
    private fun underlyingType(typeExpr: String): String {
        var current = typeExpr
        repeat(8) {
            val normalized = stripPointer(current)
            if (normalized.isBlank() || isCompositeValueType(normalized)) return current
            val type = types[resolveGoType(normalized)] ?: return current
            if (type.underlying.isBlank()) return current
            current = qualifyPackageType(type.pkg, type.underlying)
        }
        return current
    }

    fun resolveGoType(typeExpr: String): String? {
        val normalized = normalizeGoType(typeExpr)
        if (types.containsKey(normalized)) return normalized
        val matches = short[normalized].orEmpty()
        return matches.singleOrNull()
    }

    fun rangeElementType(typeExpr: String?): String? {
        val normalized = stripPointer(underlyingType(typeExpr ?: return null))
        return when {
            normalized.startsWith("[]") -> resolveGoType(normalized.removePrefix("[]"))
            normalized.startsWith("[") -> {
                val end = normalized.indexOf(']')
                if (end == -1 || end + 1 >= normalized.length) null else resolveGoType(normalized.substring(end + 1))
            }
            normalized.startsWith("map[") -> resolveMapValueType(normalized)?.let { resolveGoType(it) }
            else -> resolveGoType(normalized)
        }
    }

    fun isRangeable(typeExpr: String?): Boolean {
        val normalized = stripPointer(underlyingType(typeExpr ?: return false))
        return normalized.startsWith("[]") || normalized.startsWith("[") || normalized.startsWith("map[")
    }

    private fun resolveMapValueType(typeExpr: String): String? {
        val end = typeExpr.indexOf(']')
        if (end == -1 || end + 1 >= typeExpr.length) return null
        return typeExpr.substring(end + 1)
    }

    private fun normalizeGoType(typeExpr: String): String {
        var normalized = stripPointer(typeExpr)
        while (true) {
            normalized = stripPointer(normalized)
            normalized = when {
                normalized.startsWith("[]") -> normalized.removePrefix("[]")
                normalized.startsWith("[") -> {
                    val end = normalized.indexOf(']')
                    if (end == -1 || end + 1 >= normalized.length) return normalized
                    normalized.substring(end + 1)
                }
                else -> return normalized.trim()
            }
        }
    }

    private fun stripPointer(typeExpr: String): String {
        var normalized = typeExpr.trim()
        while (normalized.startsWith("*")) {
            normalized = normalized.removePrefix("*").trim()
        }
        return normalized
    }

    private fun isCompositeValueType(typeExpr: String): Boolean {
        val normalized = stripPointer(typeExpr)
        return normalized.startsWith("[]") || normalized.startsWith("[") || normalized.startsWith("map[")
    }

    private fun normalizeType(typeExpr: String): String {
        val lastSlash = typeExpr.lastIndexOf('/')
        val lastDot = typeExpr.lastIndexOf('.')
        return if (lastSlash > lastDot) {
            typeExpr.substring(0, lastSlash) + "." + typeExpr.substring(lastSlash + 1)
        } else {
            typeExpr
        }
    }

    private fun mergeInlineContract(filePath: String, base: TemplateContract?): TemplateContract? {
        val rawText = templateText(filePath) ?: return base
        val text = contractScanText(contractAnnotationText(rawText, base))
        val typedRoots = parseTypedRoots(text)
        val dotMatch = dotPattern.find(text)
        val funcMatches = funcPattern.findAll(text).toList()
        val genMatches = genPattern.findAll(text).toList()
        if (
            typedRoots.isEmpty() &&
            dotMatch == null &&
            funcMatches.isEmpty() &&
            genMatches.isEmpty()
        ) return base

        val roots = if (typedRoots.isEmpty()) base?.roots.orEmpty().toMutableMap() else typedRoots.associate { root ->
            root.name to (resolveGoType(root.typeName) ?: root.typeName)
        }.toMutableMap()
        val dot = dotMatch?.let { match ->
            val rawType = normalizeType(match.groupValues[1])
            resolveGoType(rawType) ?: rawType
        } ?: base?.dot.orEmpty()
        val funcs = base?.funcs.orEmpty() + funcMatches.associate { match ->
            match.groupValues[1] to normalizeType(match.groupValues[2])
        }
        val gens = (base?.gens.orEmpty() + genMatches.associate { match ->
            match.groupValues[1] to match.groupValues[2]
        })
        for ((name, _) in gens) {
            val generatedType = "$GEN_TYPE_PREFIX$name"
            if (types.containsKey(generatedType)) {
                roots[name] = generatedType
            }
        }
        return TemplateContract(roots = roots, dot = dot, funcs = funcs, gens = gens)
    }

    private fun parseTypedRoots(text: String): List<TypedRoot> {
        return annotationPattern.findAll(text).mapNotNull { match ->
            val annotation = match.groupValues[1]
            val name = match.groupValues[2]
            var typeName = match.groupValues.getOrNull(3).orEmpty()
            if (isReservedContractAnnotation(annotation)) return@mapNotNull null
            val aliasType = symbolAliases[annotation]
            if (aliasType == null && symbolStrictMode) return@mapNotNull null
            if (typeName.isBlank() && aliasType != null) typeName = aliasType
            if (typeName.isBlank()) return@mapNotNull null
            TypedRoot(annotation = annotation, name = name, typeName = typeName)
        }.toList()
    }

    private fun contractAnnotationText(text: String, base: TemplateContract?): String {
        val name = base?.name.orEmpty()
        if (name.isNotBlank()) {
            return defineBodyText(text, name) ?: text
        }
        return topLevelTemplateText(text)
    }

    private fun topLevelTemplateText(text: String): String {
        val out = StringBuilder()
        var cursor = 0
        for (block in defineBlocks(text)) {
            if (block.start > cursor) out.append(text.substring(cursor, block.start))
            cursor = maxOf(cursor, block.end)
        }
        if (cursor < text.length) out.append(text.substring(cursor))
        return out.toString()
    }

    private fun defineBodyText(text: String, name: String): String? {
        return defineBlocks(text).firstOrNull { it.name == name }?.let { text.substring(it.bodyStart, it.bodyEnd) }
    }

    // activeDefineNameAt returns the define around offset. Other blocks are
    // tracked unnamed, so their {{end}} does not close the define, and
    // examples in comments are no actions.
    private fun activeDefineNameAt(text: String, offset: Int): String? {
        val boundedOffset = offset.coerceIn(0, text.length)
        val stack = mutableListOf<String>()
        val before = text.substring(0, boundedOffset)
        for (action in GoClueTemplateActions.codeActions(before)) {
            val content = GoClueTemplateActions.content(action.text(before)) ?: continue
            val define = defineActionPattern.matchEntire(content)
            when {
                define != null -> stack.add(define.groupValues[1])
                opensBlock(content) -> stack.add("")
                content == "end" && stack.isNotEmpty() -> stack.removeAt(stack.lastIndex)
            }
        }
        return stack.lastOrNull { it.isNotEmpty() }
    }

    private fun defineBlocks(text: String): List<DefineBlock> {
        data class OpenDefine(val name: String, val start: Int, val bodyStart: Int)
        val stack = mutableListOf<OpenDefine>()
        val blocks = mutableListOf<DefineBlock>()
        for (action in GoClueTemplateActions.codeActions(text)) {
            val content = GoClueTemplateActions.content(action.text(text)) ?: continue
            val define = defineActionPattern.matchEntire(content)
            if (define != null) {
                stack.add(OpenDefine(define.groupValues[1], action.start, action.end))
                continue
            }
            if (opensBlock(content)) {
                stack.add(OpenDefine("", action.start, action.end))
                continue
            }
            if (content == "end" && stack.isNotEmpty()) {
                val open = stack.removeAt(stack.lastIndex)
                if (open.name.isNotEmpty()) {
                    blocks.add(DefineBlock(open.name, open.start, action.end, open.bodyStart, action.start))
                }
            }
        }
        return blocks.sortedBy { it.start }
    }

    // opensBlock reports whether action content starts a block other than a
    // define that a later {{end}} closes.
    private fun opensBlock(content: String): Boolean {
        return content.takeWhile { it.isLetterOrDigit() || it == '_' } in blockKeywords
    }

    private fun templateText(filePath: String): String? {
        val documentText = goClueReadAction {
            val virtualFile = LocalFileSystem.getInstance().findFileByIoFile(File(filePath))
            if (virtualFile != null) {
                FileDocumentManager.getInstance().getDocument(virtualFile)?.let { document ->
                    return@goClueReadAction document.text
                }
            }
            null
        }
        if (documentText != null) return documentText
        return try {
            File(filePath).readText()
        } catch (_: Exception) {
            null
        }
    }

    private fun contractScanText(text: String): String = GoClueTemplateActions.commentBodies(text)

    private val dotPattern = Regex("""(?m)^\s*@dot\s+([A-Za-z_][A-Za-z0-9_./\[\]*-]*)\s*$""")
    private val funcPattern = Regex("""(?m)^\s*@func\s+([A-Za-z_][A-Za-z0-9_]*)\s+([A-Za-z_][A-Za-z0-9_./-]*)\s*$""")
    private val genPattern = Regex("""(?m)^\s*@gen\s+([A-Za-z_][A-Za-z0-9_]*)\s+([A-Za-z_][A-Za-z0-9_./-]*)\s*$""")
    private val annotationPattern = Regex("""(?m)^\s*@([A-Za-z_][A-Za-z0-9_]*)\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s+([A-Za-z_][A-Za-z0-9_./\[\]*-]*))?\s*$""")
    private val blockKeywords = setOf("if", "range", "with", "block")
    private val defineActionPattern = Regex("define\\s+\"([^\"]+)\"")
    private val GEN_TYPE_PREFIX = "\$go-clue/gen."

    private data class DefineBlock(
        val name: String,
        val start: Int,
        val end: Int,
        val bodyStart: Int,
        val bodyEnd: Int,
    )

    private data class TypedRoot(
        val annotation: String,
        val name: String,
        val typeName: String,
    )

    private fun isReservedContractAnnotation(name: String): Boolean {
        return name == "dot" || name == "func" || name == "gen"
    }
}
