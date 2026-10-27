// Corre los casos de internal/engine/testdata/java/cases.tsv contra Ucum-java
// y escribe, por caso, lo que la referencia responde. No interpreta nada: la
// comparación y la clasificación las hace TestCompareJava, en Go.
// Plan docs/plans/002-compare-java.md; lo compila y corre make java-dump.
//
// Entrada (stdin), una línea por caso: id, op, source y los argumentos de op,
// separados por tabuladores. Salida (stdout), una línea por caso:
//
//   id  ok       [valor]  [código]  [precisión]  resultado
//   id  invalid  mensaje                           validate o validateInProperty rechazan
//   id  exc      clase    [mensaje]                excepción
//
// La precisión es la que el Decimal de Java declara para el valor (cuántas
// cifras significativas da por buenas). Decimal no la expone, así que se lee
// por reflexión.
//
// El mensaje solo se escribe para UcumException: el de las demás (por ejemplo
// NullPointerException) cambia entre versiones del JDK y el resultado tiene que
// ser reproducible byte a byte.

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.io.PrintStream;
import java.lang.reflect.Field;
import java.nio.charset.StandardCharsets;

import org.fhir.ucum.Decimal;
import org.fhir.ucum.Pair;
import org.fhir.ucum.UcumEssenceService;
import org.fhir.ucum.UcumException;

public class JavaDump {
  private static Field precision;

  public static void main(String[] args) throws Exception {
    precision = Decimal.class.getDeclaredField("precision");
    precision.setAccessible(true);
    UcumEssenceService s = new UcumEssenceService(JavaDump.class.getResourceAsStream("/ucum-essence.xml"));
    BufferedReader in = new BufferedReader(new InputStreamReader(System.in, StandardCharsets.UTF_8));
    PrintStream out = new PrintStream(System.out, false, "UTF-8");
    String line;
    while ((line = in.readLine()) != null) {
      String[] f = line.split("\t", -1);
      out.print(f[0]);
      out.print('\t');
      try {
        out.print(run(s, f[1], f));
      } catch (UcumException e) {
        out.print("exc\t" + e.getClass().getName() + "\t" + esc(e.getMessage()) + "\t");
      } catch (Throwable e) {
        out.print("exc\t" + e.getClass().getName() + "\t\t");
      }
      out.print('\n');
    }
    out.flush();
  }

  private static String run(UcumEssenceService s, String op, String[] f) throws Exception {
    switch (op) {
      case "validate":
        return verdict(s.validate(f[3]));
      case "inproperty":
        return verdict(s.validateInProperty(f[3], f[4]));
      case "canonical":
        return pair(s.getCanonicalForm(new Pair(new Decimal(f[3]), f[4])));
      case "convert":
        return number(s.convert(new Decimal(f[3]), f[4], f[5]), "");
      case "multiply":
        return pair(s.multiply(new Pair(new Decimal(f[3]), f[4]), new Pair(new Decimal(f[5]), f[6])));
      case "divide":
        return pair(s.divideBy(new Pair(new Decimal(f[3]), f[4]), new Pair(new Decimal(f[5]), f[6])));
      case "comparable":
        return "ok\t" + s.isComparable(f[3], f[4]) + "\t\t";
      default:
        throw new IllegalArgumentException("unknown op " + op);
    }
  }

  private static String verdict(String msg) {
    return msg == null ? "ok\t\t\t" : "invalid\t" + esc(msg) + "\t\t";
  }

  private static String pair(Pair p) throws IllegalAccessException {
    return number(p.getValue(), p.getCode());
  }

  private static String number(Decimal d, String code) throws IllegalAccessException {
    return "ok\t" + d.asDecimal() + "\t" + esc(code) + "\t" + precision.getInt(d);
  }

  private static String esc(String s) {
    if (s == null) {
      return "";
    }
    return s.replace("\\", "\\\\").replace("\t", "\\t").replace("\n", "\\n").replace("\r", "\\r");
  }
}
