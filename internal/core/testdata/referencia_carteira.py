# Planilha de referência da carteira de teste (critério de aceite da fase 4).
# Cálculo independente do Go: Decimal com 50 dígitos, XIRR por bisseção, TWR encadeado.
# Rode com: python3 internal/core/testdata/referencia_carteira.py
from datetime import date
from decimal import Decimal as D, getcontext

getcontext().prec = 50

# (data, valor de mercado no fim do dia, fluxo do dia: aporte +, resgate/provento -)
pontos = [
    (date(2025, 1, 2), D("1000.00"), D("1005.00")),   # compra 100 @ 10,00 + 5,00 de taxas
    (date(2025, 3, 31), D("1100.00"), D("0")),
    (date(2025, 4, 15), D("1710.00"), D("577.50")),   # compra 50 @ 11,50 + 2,50
    (date(2025, 6, 30), D("1800.00"), D("0")),
    (date(2025, 8, 20), D("1770.00"), D("-30.00")),   # provento de 30,00
    (date(2025, 10, 10), D("1116.00"), D("-747.00")), # venda 60 @ 12,50 − 3,00
    (date(2025, 12, 31), D("1170.00"), D("0")),
]

cota, anterior = D(1), D(0)
for _, v, f in pontos:
    base = anterior + f
    if base > 0:
        cota *= v / base
    anterior = v
twr = cota - 1

fluxos = [(d, -f) for d, _, f in pontos if f != 0] + [(pontos[-1][0], pontos[-1][1])]
d0 = fluxos[0][0]

def vpl(taxa):
    return sum(v / (1 + taxa) ** (D((d - d0).days) / D(365)) for d, v in fluxos)

lo, hi = D("-0.99"), D("10")
for _ in range(200):
    meio = (lo + hi) / 2
    if (vpl(meio) > 0) == (vpl(lo) > 0):
        lo = meio
    else:
        hi = meio
xirr = (lo + hi) / 2

print(f"TWR  = {twr * 100:.6f} %")
print(f"XIRR = {xirr * 100:.6f} % a.a.")
