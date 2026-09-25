import { Construction } from "lucide-react";
import { Cartao, Pagina, Vazio } from "../components/ui";

export function EmBreve({ titulo, pergunta, fase }: { titulo: string; pergunta: string; fase: string }) {
  return (
    <Pagina titulo={titulo} subtitulo={pergunta}>
      <Cartao>
        <Vazio
          icone={<Construction className="size-8" />}
          titulo={`Chega na ${fase}`}
          texto="A fundação (login, casas, entidades e privacidade) vem primeiro."
        />
      </Cartao>
    </Pagina>
  );
}
