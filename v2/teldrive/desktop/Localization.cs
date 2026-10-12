using System.ComponentModel;
using System.Globalization;
using System.Text.Json;
using System.Text.RegularExpressions;
using System.Windows;
using System.Windows.Data;
using System.Windows.Markup;

namespace Teldrive.Desktop;

public sealed record DesktopLanguage(string Id, string Name);
public sealed class L : INotifyPropertyChanged
{
    public static L Current { get; } = new();
    public static DesktopLanguage[] Languages { get; } = [new("es", "Español"), new("en", "English"), new("pt", "Português"), new("fr", "Français"), new("de", "Deutsch"), new("it", "Italiano"), new("ru", "Русский"), new("zh-CN", "简体中文"), new("ja", "日本語"), new("ar", "العربية")];
    private readonly Dictionary<string, Dictionary<string, string>> catalog;
    private string language = "es";
    public L()
    {
        using var stream = typeof(L).Assembly.GetManifestResourceStream("Teldrive.Desktop.LocalizationCatalog") ?? throw new InvalidOperationException("Missing localization catalog");
        catalog = JsonSerializer.Deserialize<Dictionary<string, Dictionary<string, string>>>(stream)!;
    }
    public string Language {
        get => language;
        set { var selected = Languages.Any(item => item.Id == value) ? value : "es"; if (language == selected) return; language = selected; PropertyChanged?.Invoke(this, new(nameof(Language))); }
    }
    public event PropertyChangedEventHandler? PropertyChanged;
    public static string T(string source, params object?[] values)
    {
        var dictionary = Current.catalog[Current.Language];
        var text = dictionary.GetValueOrDefault(source, source);
        return Regex.Replace(text, @"\{(\d+)\}", match => int.Parse(match.Groups[1].Value, CultureInfo.InvariantCulture) is var index && index < values.Length ? Convert.ToString(values[index], CultureInfo.CurrentCulture) ?? "" : match.Value);
    }
}
public sealed class LocalizeExtension : MarkupExtension, IValueConverter
{
    public string Text { get; set; } = "";
    public override object ProvideValue(IServiceProvider serviceProvider) => new Binding(nameof(L.Language)) { Source = L.Current, Converter = this, Mode = BindingMode.OneWay }.ProvideValue(serviceProvider);
    public object Convert(object value, Type targetType, object parameter, CultureInfo culture) => L.T(Text);
    public object ConvertBack(object value, Type targetType, object parameter, CultureInfo culture) => DependencyProperty.UnsetValue;
}
