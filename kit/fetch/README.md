# fetch

Descarga de archivos a un temporal.

## Uso

```go
path, err := fetch.ToTemp(url)
```

`ToTemp` baja el archivo, lo deja ejecutable y devuelve la ruta. Si
la descarga falla, borra el parcial.

## Por qué el `?cb=` en la URL

```go
full := fmt.Sprintf("%s?cb=%d", url, time.Now().Unix())
```

El CDN de GitHub cachea ~5 minutos por path. Un `?cb=` con el
timestamp hace que la URL sea única y el CDN no sirva la versión
cacheada. Es lo que evita el "la DB vieja con la firma nueva" que
rompió el `pacman -Sy` del repo.

## Timeout

5 minutos, con 15 segundos de connect timeout. El `curl` va con
`-fsSL`: falla en errores HTTP en vez de guardar la página de error
como si fuera el archivo.
