// Sonda del plan docs/plans/023-strict-xml-values.md, §5.5: carga ucum-essence.xml
// con cada literal puesto en el value="5280" de [mi_i] y escribe si Ucum-java lo
// acepta y cuánto vale 1 [mi_i] en forma canónica. No interpreta nada: la
// comparación con decimal.FromString la hace un test en Go.
//
// Uso: java JavaValues <ucum-essence.xml> < literales.txt
//
// Entrada (stdin), un literal por línea, con escapes de barra (tabulador, barra y u más cuatro cifras hex) para lo
// que no cabe en una línea. En el XML, todo carácter que no sea alfanumérico,
// punto o signo va como referencia numérica (&#N;), porque un tabulador literal
// en un atributo se normaliza a espacio.
//
// Salida (stdout), una línea por literal:
//
//   literal  accept  valor de 1 [mi_i] en metros
//   literal  reject  (sin valor: los mensajes de Java no son estables)
//
// El literal se reescribe con los mismos escapes. La salida es determinista.
//
// Java no termina con exponentes grandes (1e9999) ni con dígitos no ASCII (el
// valor del dígito sale de restar '0'): esos literales no se le pasan.

import java.io.BufferedReader;
import java.io.ByteArrayInputStream;
import java.io.InputStreamReader;
import java.io.PrintStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Paths;

import org.fhir.ucum.Decimal;
import org.fhir.ucum.Pair;
import org.fhir.ucum.UcumEssenceService;

public class JavaValues {
  private static final String ANCHOR = "<value Unit=\"[ft_i]\" UNIT=\"[FT_I]\" value=\"5280\">";

  public static void main(String[] args) throws Exception {
    String xml = new String(Files.readAllBytes(Paths.get(args[0])), StandardCharsets.UTF_8);
    if (xml.indexOf(ANCHOR) < 0 || xml.indexOf(ANCHOR) != xml.lastIndexOf(ANCHOR)) {
      throw new IllegalStateException("anchor of [mi_i] not found exactly once");
    }
    BufferedReader in = new BufferedReader(new InputStreamReader(System.in, StandardCharsets.UTF_8));
    PrintStream out = new PrintStream(System.out, false, "UTF-8");
    String line;
    while ((line = in.readLine()) != null) {
      String lit = unesc(line);
      String doc = xml.replace(ANCHOR,
          "<value Unit=\"[ft_i]\" UNIT=\"[FT_I]\" value=\"" + attr(lit) + "\">");
      out.print(esc(lit));
      try {
        UcumEssenceService s = new UcumEssenceService(new ByteArrayInputStream(doc.getBytes(StandardCharsets.UTF_8)));
        Pair p = s.getCanonicalForm(new Pair(new Decimal("1"), "[mi_i]"));
        out.print("\taccept\t" + p.getValue().asDecimal());
      } catch (Throwable e) {
        out.print("\treject\t");
      }
      out.print('\n');
      out.flush();
    }
    out.flush();
  }

  private static String attr(String s) {
    StringBuilder b = new StringBuilder();
    s.codePoints().forEach(c -> {
      if ((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '.' || c == '-') {
        b.append((char) c);
      } else {
        b.append("&#").append(c).append(';');
      }
    });
    return b.toString();
  }

  private static String unesc(String s) {
    StringBuilder b = new StringBuilder();
    for (int i = 0; i < s.length(); i++) {
      char c = s.charAt(i);
      if (c != '\\') {
        b.append(c);
      } else if (s.charAt(i + 1) == 'u') {
        b.append((char) Integer.parseInt(s.substring(i + 2, i + 6), 16));
        i += 5;
      } else {
        b.append(s.charAt(i + 1) == 't' ? '\t' : s.charAt(i + 1));
        i++;
      }
    }
    return b.toString();
  }

  private static String esc(String s) {
    StringBuilder b = new StringBuilder();
    for (char c : s.toCharArray()) {
      if (c == '\\') {
        b.append("\\\\");
      } else if (c == '\t') {
        b.append("\\t");
      } else if (c == '\n' || c == '\r') {
        b.append(' ');
      } else if (c > 126 || c < 32) {
        b.append(String.format("\\u%04x", (int) c));
      } else {
        b.append(c);
      }
    }
    return b.toString();
  }
}
